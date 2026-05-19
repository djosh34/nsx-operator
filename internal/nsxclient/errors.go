package nsxclient

import "fmt"

type WriteDisabledReason string

const (
	WriteDisabledReasonGlobalConfig WriteDisabledReason = "global_config"
	WriteDisabledReasonNetworkCloud WriteDisabledReason = "network_cloud"
)

type WriteDisabledError struct {
	Method           string
	URL              string
	Reason           WriteDisabledReason
	NetworkCloudName string
	NetworkCloudFQDN string
}

func (err WriteDisabledError) Error() string {
	if err.Reason == "" {
		return fmt.Sprintf("nsx %s %s skipped because writes are disabled", err.Method, err.URL)
	}
	return fmt.Sprintf("nsx %s %s skipped because writes are disabled by %s", err.Method, err.URL, err.Reason)
}

type StatusError struct {
	StatusCode int
	Method     string
	URL        string
	Body       string
}

func (err StatusError) Error() string {
	if err.Body == "" {
		return fmt.Sprintf("nsx %s %s returned status %d", err.Method, err.URL, err.StatusCode)
	}
	return fmt.Sprintf("nsx %s %s returned status %d: %s", err.Method, err.URL, err.StatusCode, err.Body)
}

type ConflictError struct {
	StatusError
}

type PreconditionFailedError struct {
	StatusError
}

type RateLimitedError struct {
	StatusError
}

type ServiceUnavailableError struct {
	StatusError
}
