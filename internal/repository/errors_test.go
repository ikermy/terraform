package repository

import (
	"errors"
	"net/http"
	"testing"

	"terraform-provider-ai/internal/entity"
)

func TestAPIError_IsNotFound(t *testing.T) {
	err := APIErrorFor(http.MethodGet, "/clusters/x", http.StatusNotFound, "not found", entity.ErrClusterNotFound)
	if !errors.Is(err, entity.ErrClusterNotFound) {
		t.Fatal("expected errors.Is to match entity.ErrClusterNotFound")
	}
	if !err.IsNotFound() {
		t.Fatal("expected IsNotFound to be true")
	}
	if err.IsRetryable() {
		t.Fatal("404 must not be retryable")
	}
	if err.IsUnauthorized() {
		t.Fatal("404 must not be unauthorized")
	}
}

func TestAPIError_IsUnauthorized(t *testing.T) {
	err := APIErrorFor(http.MethodGet, "/clusters/x", http.StatusUnauthorized, "unauthorized", nil)
	if !err.IsUnauthorized() {
		t.Fatal("expected IsUnauthorized to be true")
	}
	if err.IsRetryable() {
		t.Fatal("401 must not be retryable")
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		err := APIErrorFor(http.MethodPost, "/clusters", status, "upstream", nil)
		if !err.IsRetryable() {
			t.Fatalf("expected status %d to be retryable", status)
		}
	}
}

func TestAPIError_UnwrapNilCause(t *testing.T) {
	err := APIErrorFor(http.MethodGet, "/clusters/x", http.StatusBadRequest, "bad request", nil)
	if errors.Is(err, entity.ErrClusterNotFound) {
		t.Fatal("did not expect errors.Is to match when cause is nil")
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("expected Unwrap to return nil")
	}
}

func TestAPIError_ErrorFormatting(t *testing.T) {
	err := APIErrorFor(http.MethodGet, "/clusters/x", http.StatusNotFound, "not found", entity.ErrClusterNotFound)
	got := err.Error()
	want := "GET /clusters/x: 404 not found"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
