package nsxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/djosh34/nsx-operator/internal/operatormetrics"
	"go.uber.org/zap"
)

const (
	defaultDomainID      = "default"
	maxStatusBodyPreview = 64 * 1024
)

// Options configures an NSX manager client.
type Options struct {
	BaseURL      string
	HTTPClient   *http.Client
	Username     string
	Password     string
	Logger       *zap.Logger
	WriteControl WriteControl
	Recorder     operatormetrics.Recorder
}

// WriteControl controls whether mutating NSX requests are allowed.
type WriteControl struct {
	Enabled          bool
	Reason           WriteDisabledReason
	NetworkCloudName string
	NetworkCloudFQDN string
}

// Client performs authenticated NSX manager API calls.
type Client struct {
	baseURL      *url.URL
	httpClient   *http.Client
	username     string
	password     string
	log          *zap.Logger
	writeControl WriteControl
	recorder     operatormetrics.Recorder
}

// NewClient constructs an NSX manager client.
//
//nolint:gocritic // public constructor keeps value options so callers can pass literals.
func NewClient(options Options) (*Client, error) {
	if options.BaseURL == "" {
		return nil, errors.New("nsx base url is required")
	}
	baseURL, err := url.Parse(options.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse nsx base url: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, fmt.Errorf("nsx base url must be absolute http or https URL")
	}
	if options.Username == "" {
		return nil, errors.New("nsx basic auth username is required")
	}
	if options.Password == "" {
		return nil, errors.New("nsx basic auth password is required")
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	log := options.Logger
	if log == nil {
		log = zap.NewNop()
	}
	recorder := options.Recorder
	if recorder == nil {
		recorder = operatormetrics.NopRecorder{}
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")

	writeControl := options.WriteControl
	if writeControl.Reason == "" {
		writeControl.Enabled = true
	}

	log.Info(
		"constructed nsx manager client",
		zap.String("baseURL", redactedURL(baseURL)),
		zap.Bool("nsxWritesEnabled", writeControl.Enabled),
		zap.String("writeDisabledReason", string(writeControl.Reason)),
		zap.String("networkCloudName", writeControl.NetworkCloudName),
		zap.String("networkCloudFQDN", writeControl.NetworkCloudFQDN),
	)
	return &Client{
		baseURL:      baseURL,
		httpClient:   httpClient,
		username:     options.Username,
		password:     options.Password,
		log:          log,
		writeControl: writeControl,
		recorder:     recorder,
	}, nil
}

// DecodeListResults decodes one NSX paginated list response.
func DecodeListResults[T any](reader io.Reader) ([]*T, string, int, error) {
	var page struct {
		Results     []json.RawMessage `json:"results"`
		Cursor      string            `json:"cursor"`
		ResultCount int               `json:"result_count"`
	}
	err := json.NewDecoder(reader).Decode(&page)
	if err != nil {
		return nil, "", 0, fmt.Errorf("decode nsx list result: %w", err)
	}
	results := make([]*T, 0, len(page.Results))
	for index, raw := range page.Results {
		var item T
		err = json.Unmarshal(raw, &item)
		if err != nil {
			return nil, "", 0, fmt.Errorf("decode nsx list result item %d: %w", index, err)
		}
		results = append(results, &item)
	}
	return results, page.Cursor, page.ResultCount, nil
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, payload any, target any) error {
	err := c.requireWriteEnabled(method, path, query)
	if err != nil {
		return err
	}
	function := nsxFunction(method, path, query)
	manager := c.metricsManager()
	c.recorder.ObserveNSXCall(manager, function)
	req, err := c.newRequest(ctx, method, path, query, payload)
	if err != nil {
		return err
	}
	requestBytes := requestBodyBytes(req)

	log := c.log.With(
		zap.String("method", method),
		zap.String("url", redactedURL(req.URL)),
		zap.String("function", function),
		zap.String("manager", manager),
	)
	log.Debug("sending nsx request")
	start := time.Now()
	//nolint:bodyclose // handleResponse always closes non-nil response bodies and joins close errors.
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.recorder.ObserveNSXHTTP(manager, function, requestBytes, 0, time.Since(start))
		log.Debug("nsx request failed", zap.Error(err))
		return fmt.Errorf("send nsx request: %w", err)
	}
	body := wrapCountingBody(resp)
	handleErr := c.handleResponse(resp, target)
	c.recorder.ObserveNSXHTTP(manager, function, requestBytes, body.bytesRead(), time.Since(start))
	if handleErr != nil {
		log.Debug("nsx response failed", zap.Error(handleErr))
		return handleErr
	}
	log.Debug("completed nsx request", zap.Int("statusCode", resp.StatusCode))
	return nil
}

func (c *Client) requireWriteEnabled(method string, path string, query url.Values) error {
	if method == http.MethodGet || c.writeControl.Enabled {
		return nil
	}
	requestURL := c.requestURL(path, query)
	writeErr := WriteDisabledError{
		Method:           method,
		URL:              redactedURL(&requestURL),
		Reason:           c.writeControl.Reason,
		NetworkCloudName: c.writeControl.NetworkCloudName,
		NetworkCloudFQDN: c.writeControl.NetworkCloudFQDN,
	}
	c.log.Info(
		"skipped nsx write request because writes are disabled",
		zap.String("method", method),
		zap.String("url", writeErr.URL),
		zap.String("path", path),
		zap.String("writeDisabledReason", string(writeErr.Reason)),
		zap.String("networkCloudName", writeErr.NetworkCloudName),
		zap.String("networkCloudFQDN", writeErr.NetworkCloudFQDN),
	)
	return writeErr
}

func (c *Client) requestURL(path string, query url.Values) url.URL {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	requestURL.RawQuery = query.Encode()
	return requestURL
}

func (c *Client) newRequest(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	payload any,
) (*http.Request, error) {
	requestURL := c.requestURL(path, query)

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode nsx request body: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create nsx request: %w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) handleResponse(resp *http.Response, target any) (retErr error) {
	if resp == nil {
		return errors.New("nsx response is nil")
	}
	if resp.Body == nil {
		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			return nil
		}
		return statusError(resp, "")
	}

	var resultErr error
	defer func() {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close nsx response body: %w", closeErr))
		}
		retErr = errors.Join(retErr, resultErr)
	}()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		preview, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusBodyPreview))
		if err != nil {
			return fmt.Errorf("read nsx status response body: %w", err)
		}
		return statusError(resp, strings.TrimSpace(string(preview)))
	}

	if target == nil {
		_, err := io.Copy(io.Discard, resp.Body)
		if err != nil {
			return fmt.Errorf("drain nsx response body: %w", err)
		}
		return nil
	}
	err := json.NewDecoder(resp.Body).Decode(target)
	if err != nil {
		return fmt.Errorf("decode nsx response body: %w", err)
	}
	return nil
}

func listAllTyped[T any](ctx context.Context, c *Client, path string, query url.Values) ([]*T, error) {
	accumulated := []*T{}
	cursor := ""
	function := nsxFunction(http.MethodGet, path, query)
	manager := c.metricsManager()
	c.recorder.ObserveNSXCall(manager, function)
	for {
		pageQuery := cloneValues(query)
		if cursor != "" {
			pageQuery.Set("cursor", cursor)
		}
		req, err := c.newRequest(ctx, http.MethodGet, path, pageQuery, nil)
		if err != nil {
			return nil, err
		}
		requestBytes := requestBodyBytes(req)
		c.log.Debug("sending nsx list request", zap.String("url", redactedURL(req.URL)), zap.String("function", function), zap.String("manager", manager))
		start := time.Now()
		//nolint:bodyclose // decodeAndCloseList or handleResponse closes non-nil response bodies on every path.
		resp, err := c.httpClient.Do(req)
		if err != nil {
			c.recorder.ObserveNSXHTTP(manager, function, requestBytes, 0, time.Since(start))
			return nil, fmt.Errorf("send nsx list request: %w", err)
		}
		body := wrapCountingBody(resp)
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			handleErr := c.handleResponse(resp, nil)
			if handleErr != nil {
				c.recorder.ObserveNSXHTTP(manager, function, requestBytes, body.bytesRead(), time.Since(start))
				return nil, handleErr
			}
			c.recorder.ObserveNSXHTTP(manager, function, requestBytes, body.bytesRead(), time.Since(start))
			return nil, fmt.Errorf("nsx list request returned status %d without error", resp.StatusCode)
		}
		items, nextCursor, _, err := decodeAndCloseList[T](resp.Body)
		c.recorder.ObserveNSXHTTP(manager, function, requestBytes, body.bytesRead(), time.Since(start))
		if err != nil {
			return nil, err
		}
		accumulated = append(accumulated, items...)
		if nextCursor == "" {
			return accumulated, nil
		}
		cursor = nextCursor
	}
}

func (c *Client) metricsManager() string {
	if c.writeControl.NetworkCloudFQDN != "" {
		return c.writeControl.NetworkCloudFQDN
	}
	return c.baseURL.Host
}

func requestBodyBytes(req *http.Request) int64 {
	if req == nil || req.ContentLength <= 0 {
		return 0
	}
	return req.ContentLength
}

func wrapCountingBody(resp *http.Response) *countingReadCloser {
	if resp == nil || resp.Body == nil {
		return &countingReadCloser{}
	}
	counting := &countingReadCloser{ReadCloser: resp.Body}
	resp.Body = counting
	return counting
}

type countingReadCloser struct {
	io.ReadCloser
	count int64
}

func (body *countingReadCloser) Read(p []byte) (int, error) {
	if body == nil || body.ReadCloser == nil {
		return 0, io.EOF
	}
	n, err := body.ReadCloser.Read(p)
	body.count += int64(n)
	return n, err
}

func (body *countingReadCloser) Close() error {
	if body == nil || body.ReadCloser == nil {
		return nil
	}
	return body.ReadCloser.Close()
}

func (body *countingReadCloser) bytesRead() int64 {
	if body == nil {
		return 0
	}
	return body.count
}

func decodeAndCloseList[T any](body io.ReadCloser) ([]*T, string, int, error) {
	results, cursor, count, decodeErr := DecodeListResults[T](body)
	closeErr := body.Close()
	if decodeErr != nil && closeErr != nil {
		return nil, "", 0, errors.Join(decodeErr, fmt.Errorf("close nsx list response body: %w", closeErr))
	}
	if decodeErr != nil {
		return nil, "", 0, decodeErr
	}
	if closeErr != nil {
		return nil, "", 0, fmt.Errorf("close nsx list response body: %w", closeErr)
	}
	return results, cursor, count, nil
}

func statusError(resp *http.Response, body string) error {
	req := resp.Request
	method := ""
	requestURL := ""
	if req != nil {
		method = req.Method
		if req.URL != nil {
			requestURL = redactedURL(req.URL)
		}
	}
	base := StatusError{
		StatusCode: resp.StatusCode,
		Method:     method,
		URL:        requestURL,
		Body:       body,
	}
	switch resp.StatusCode {
	case http.StatusConflict:
		return ConflictError{StatusError: base}
	case http.StatusPreconditionFailed:
		return PreconditionFailedError{StatusError: base}
	case http.StatusTooManyRequests:
		return RateLimitedError{StatusError: base}
	case http.StatusServiceUnavailable:
		return ServiceUnavailableError{StatusError: base}
	default:
		return base
	}
}

func actionQuery(action string) url.Values {
	values := url.Values{}
	if action != "" {
		values.Set("action", action)
	}
	return values
}

func cloneValues(values url.Values) url.Values {
	cloned := url.Values{}
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}

func nsxFunction(method string, path string, query url.Values) string {
	action := ""
	if query != nil {
		action = query.Get("action")
	}
	switch {
	case method == http.MethodGet && path == defaultDomainPath()+"/groups":
		return "list_groups"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && pathHasSuffix(path, "/members/ip-addresses"):
		return "list_group_ip_address_members"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && pathHasSuffix(path, "/members/ip-groups"):
		return "list_group_ip_group_members"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && pathHasSuffix(path, "/members/segments"):
		return "list_group_segment_members"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && strings.Contains(path, "/ip-address-expressions/"):
		return methodActionFunction(method, action, "group_ip_address_expression")
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && strings.Contains(path, "/path-expressions/"):
		return methodActionFunction(method, action, "group_path_expression")
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/") && pathHasSuffix(path, "/members/consolidated-effective-ip-addresses"):
		return "get_global_consolidated_effective_ip_addresses"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/groups/"):
		return methodFunction(method, "group")
	case method == http.MethodGet && path == "/policy/api/v1/eula/acceptance":
		return "get_eula_acceptance"
	case path == "/api/v1/search/query":
		return "search_manager_query"
	case path == "/api/v1/search/dsl":
		return "search_manager_dsl"
	case path == "/policy/api/v1/search/query":
		return "search_policy_query"
	case path == "/policy/api/v1/search/dsl":
		return "search_policy_dsl"
	case path == "/api/v1/firewall/sections":
		return methodActionFunction(method, action, "firewall_section")
	case pathHasPrefixSegments(path, "/api/v1/firewall/sections/") && strings.Contains(path, "/rules/"):
		return methodActionFunction(method, action, "firewall_rule")
	case pathHasPrefixSegments(path, "/api/v1/firewall/sections/") && pathHasSuffix(path, "/rules"):
		return methodActionFunction(method, action, "firewall_rule")
	case pathHasPrefixSegments(path, "/api/v1/firewall/sections/") && pathHasSuffix(path, "/rules/stats"):
		return "list_firewall_rule_stats"
	case pathHasPrefixSegments(path, "/api/v1/firewall/sections/"):
		return methodActionFunction(method, action, "firewall_section")
	case path == "/api/v1/ip-sets":
		return methodFunction(method, "ip_set")
	case pathHasPrefixSegments(path, "/api/v1/ip-sets/") && pathHasSuffix(path, "/members"):
		return "list_ip_set_members"
	case pathHasPrefixSegments(path, "/api/v1/ip-sets/"):
		return methodActionFunction(method, action, "ip_set")
	case path == defaultDomainPath()+"/security-policies":
		return "list_security_policies"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/security-policies/") && strings.Contains(path, "/rules/"):
		return methodActionFunction(method, action, "security_rule")
	case pathHasPrefixSegments(path, defaultDomainPath()+"/security-policies/") && pathHasSuffix(path, "/rules"):
		return "list_security_rules"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/security-policies/") && pathHasSuffix(path, "/statistics"):
		return "list_security_policy_stats"
	case pathHasPrefixSegments(path, defaultDomainPath()+"/security-policies/"):
		return methodActionFunction(method, action, "security_policy")
	case path == "/policy/api/v1/infra/segments":
		return "list_infra_segments"
	case path == "/policy/api/v1/infra/segments/state":
		return "list_infra_segment_states"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/segments/") && pathHasSuffix(path, "/state"):
		return "get_infra_segment_state"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/segments/") && pathHasSuffix(path, "/statistics"):
		return "get_infra_segment_statistics"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/segments/"):
		return methodFunction(method, "infra_segment")
	case path == "/policy/api/v1/infra/tier-0s":
		return "list_tier0s"
	case path == "/policy/api/v1/infra/tier-1s":
		return "list_tier1s"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && strings.Contains(path, "/segments/") && pathHasSuffix(path, "/state"):
		return "get_tier1_segment_state"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && strings.Contains(path, "/segments/") && pathHasSuffix(path, "/statistics"):
		return "get_tier1_segment_statistics"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && pathHasSuffix(path, "/segments/state"):
		return "list_tier1_segment_states"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && pathHasSuffix(path, "/segments"):
		return "list_tier1_segments"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && strings.Contains(path, "/segments/"):
		return methodFunction(method, "tier1_segment")
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/") && pathHasSuffix(path, "/state"):
		return "get_tier1_state"
	case pathHasPrefixSegments(path, "/policy/api/v1/infra/tier-1s/"):
		return methodFunction(method, "tier1")
	case pathHasPrefixSegments(path, "/policy/api/v1/global-infra/tier-1s/") && pathHasSuffix(path, "/state"):
		return "get_global_tier1_segment_state"
	case pathHasPrefixSegments(path, "/policy/api/v1/global-infra/tier-1s/") && pathHasSuffix(path, "/statistics"):
		return "get_global_tier1_segment_statistics"
	default:
		return methodFunction(method, "unknown")
	}
}

func methodActionFunction(method string, action string, resource string) string {
	if action != "" {
		return action + "_" + resource
	}
	return methodFunction(method, resource)
}

func methodFunction(method string, resource string) string {
	switch method {
	case http.MethodGet:
		return "get_" + resource
	case http.MethodPost:
		return "create_" + resource
	case http.MethodPut:
		return "put_" + resource
	case http.MethodPatch:
		return "patch_" + resource
	case http.MethodDelete:
		return "delete_" + resource
	default:
		return strings.ToLower(method) + "_" + resource
	}
}

func pathHasPrefixSegments(path string, prefix string) bool {
	return strings.HasPrefix(path, prefix)
}

func pathHasSuffix(path string, suffix string) bool {
	return strings.HasSuffix(path, suffix)
}

func pathEscape(value string) string {
	return url.PathEscape(value)
}

func redactedURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	redacted := *value
	if redacted.User != nil {
		redacted.User = url.UserPassword(redacted.User.Username(), "xxxxx")
	}
	return redacted.String()
}
