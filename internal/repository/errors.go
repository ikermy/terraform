package repository

import (
	"fmt"
	"net/http"
)

// APIError is an infrastructure-level error that carries the HTTP context
// (status code, method, path). It wraps an optional domain sentinel via
// Unwrap, so callers can check errors.Is(err, entity.ErrClusterNotFound)
// while still having access to the transport details.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Message    string
	cause      error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error { return e.cause }

func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

func (e *APIError) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

func (e *APIError) IsRetryable() bool {
	return e.StatusCode >= 500 || e.StatusCode == http.StatusTooManyRequests
}

// APIErrorFor builds an APIError for a transport-level failure, optionally
// binding it to a domain sentinel so errors.Is works up the stack.
func APIErrorFor(method, path string, status int, message string, cause error) *APIError {
	return &APIError{
		StatusCode: status,
		Method:     method,
		Path:       path,
		Message:    message,
		cause:      cause,
	}
}
