package nsxclient

import "fmt"

// WriteDisabledReason identifies why mutating NSX writes are disabled.
type WriteDisabledReason string

// WriteDisabledReason constants identify supported write-disable sources.
const (
	WriteDisabledReasonGlobalConfig WriteDisabledReason = "global_config"
	WriteDisabledReasonNetworkCloud WriteDisabledReason = "network_cloud"
)

// WriteDisabledError reports a skipped mutating request while writes are disabled.
type WriteDisabledError struct {
	Method           string
	URL              string
	Reason           WriteDisabledReason
	NetworkCloudName string
	NetworkCloudFQDN string
}

func (err *WriteDisabledError) Error() string {
	if err == nil {
		return "nsx write disabled"
	}
	if err.Reason == "" {
		return fmt.Sprintf("nsx %s %s skipped because writes are disabled", err.Method, err.URL)
	}
	return fmt.Sprintf("nsx %s %s skipped because writes are disabled by %s", err.Method, err.URL, err.Reason)
}

// StatusError reports an unexpected NSX HTTP status response.
type StatusError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (err *StatusError) Error() string {
	if err == nil {
		return "nsx returned unexpected status"
	}
	if err.Body == "" {
		return fmt.Sprintf("nsx %s %s returned status %d", err.Method, err.URL, err.StatusCode)
	}
	return fmt.Sprintf("nsx %s %s returned status %d: %s", err.Method, err.URL, err.StatusCode, err.Body)
}

// ConflictError reports an NSX 409 response.
type ConflictError struct {
	StatusError
}

// PreconditionFailedError reports an NSX 412 response.
type PreconditionFailedError struct {
	StatusError
}

// RateLimitedError reports an NSX 429 response.
type RateLimitedError struct {
	StatusError
}

// ServiceUnavailableError reports an NSX 503 response.
type ServiceUnavailableError struct {
	StatusError
}
