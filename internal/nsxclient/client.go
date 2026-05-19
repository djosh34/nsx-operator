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

	"go.uber.org/zap"
)

const (
	defaultDomainID      = "default"
	maxStatusBodyPreview = 64 * 1024
)

type Options struct {
	BaseURL      string
	HTTPClient   *http.Client
	Username     string
	Password     string
	Logger       *zap.Logger
	WriteControl WriteControl
}

type WriteControl struct {
	Enabled          bool
	Reason           WriteDisabledReason
	NetworkCloudName string
	NetworkCloudFQDN string
}

type Client struct {
	baseURL      *url.URL
	httpClient   *http.Client
	username     string
	password     string
	log          *zap.Logger
	writeControl WriteControl
}

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
	}, nil
}

func DecodeListResults[T any](reader io.Reader) ([]*T, string, int, error) {
	var page struct {
		Results     []json.RawMessage `json:"results"`
		Cursor      string            `json:"cursor"`
		ResultCount int               `json:"result_count"`
	}
	if err := json.NewDecoder(reader).Decode(&page); err != nil {
		return nil, "", 0, fmt.Errorf("decode nsx list result: %w", err)
	}
	results := make([]*T, 0, len(page.Results))
	for index, raw := range page.Results {
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, "", 0, fmt.Errorf("decode nsx list result item %d: %w", index, err)
		}
		results = append(results, &item)
	}
	return results, page.Cursor, page.ResultCount, nil
}

func (c *Client) do(ctx context.Context, method string, path string, query url.Values, payload any, target any) error {
	if err := c.requireWriteEnabled(method, path, query); err != nil {
		return err
	}
	req, err := c.newRequest(ctx, method, path, query, payload)
	if err != nil {
		return err
	}

	log := c.log.With(
		zap.String("method", method),
		zap.String("url", redactedURL(req.URL)),
	)
	log.Debug("sending nsx request")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Debug("nsx request failed", zap.Error(err))
		return fmt.Errorf("send nsx request: %w", err)
	}
	handleErr := c.handleResponse(resp, target)
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
		if closeErr := resp.Body.Close(); closeErr != nil {
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
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return fmt.Errorf("drain nsx response body: %w", err)
		}
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode nsx response body: %w", err)
	}
	return nil
}

func listAllTyped[T any](ctx context.Context, c *Client, path string, query url.Values) ([]*T, error) {
	accumulated := []*T{}
	cursor := ""
	for {
		pageQuery := cloneValues(query)
		if cursor != "" {
			pageQuery.Set("cursor", cursor)
		}
		req, err := c.newRequest(ctx, http.MethodGet, path, pageQuery, nil)
		if err != nil {
			return nil, err
		}
		c.log.Debug("sending nsx list request", zap.String("url", redactedURL(req.URL)))
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("send nsx list request: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			if handleErr := c.handleResponse(resp, nil); handleErr != nil {
				return nil, handleErr
			}
			return nil, fmt.Errorf("nsx list request returned status %d without error", resp.StatusCode)
		}
		items, nextCursor, _, err := decodeAndCloseList[T](resp.Body)
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
