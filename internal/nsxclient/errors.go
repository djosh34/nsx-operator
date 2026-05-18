package nsxclient

import "fmt"

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
