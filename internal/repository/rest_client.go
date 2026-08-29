package repository

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

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"terraform-provider-ai/internal/entity"
)

const (
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
	baseBackoff    = 200 * time.Millisecond
)

type RestClient struct {
	endpoint string
	apiToken string
	hc       *http.Client
}

type ClientOption func(*RestClient)

// WithHTTPClient lets callers inject a custom http.Client
// (transport, timeouts, mocks) instead of the default one.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *RestClient) { c.hc = hc }
}

func NewRestClient(endpoint, apiToken string, opts ...ClientOption) *RestClient {
	c := &RestClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		apiToken: apiToken,
		hc:       &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *RestClient) Create(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error) {
	body, err := json.Marshal(cluster)
	if err != nil {
		return nil, err
	}

	path, err := url.JoinPath(c.endpoint, "clusters")
	if err != nil {
		return nil, err
	}

	var created entity.Cluster
	// Idempotency-Key makes Create safe to retry: the server returns the
	// previously created cluster for the same key instead of duplicating it.
	headers := map[string]string{"Idempotency-Key": cluster.Name}
	err = c.do(ctx, http.MethodPost, path, body, headers, &created, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	return &created, nil
}

func (c *RestClient) Get(ctx context.Context, id string) (*entity.Cluster, error) {
	path, err := url.JoinPath(c.endpoint, "clusters", id)
	if err != nil {
		return nil, err
	}

	var cluster entity.Cluster
	err = c.do(ctx, http.MethodGet, path, nil, nil, &cluster, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	return &cluster, nil
}

func (c *RestClient) Update(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error) {
	body, err := json.Marshal(cluster)
	if err != nil {
		return nil, err
	}

	path, err := url.JoinPath(c.endpoint, "clusters", cluster.ID)
	if err != nil {
		return nil, err
	}

	var updated entity.Cluster
	err = c.do(ctx, http.MethodPut, path, body, nil, &updated, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("update: %w", err)
	}
	return &updated, nil
}

func (c *RestClient) Delete(ctx context.Context, id string) error {
	path, err := url.JoinPath(c.endpoint, "clusters", id)
	if err != nil {
		return err
	}

	err = c.do(ctx, http.MethodDelete, path, nil, nil, nil, http.StatusNoContent)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}

// do sends an HTTP request with retry on transient errors and classifies
// responses: 2xx success, 404 -> entity.ErrClusterNotFound, 401/403 ->
// unauthorized, other 4xx -> unhandled client error, 5xx/429 -> retry.
func (c *RestClient) do(ctx context.Context, method, path string, body []byte, headers map[string]string, out any, wantStatus int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			tflog.Debug(ctx, "retrying request", map[string]any{
				"attempt": attempt,
				"max":     maxRetries,
			})
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return err
			}
		}

		lastErr = c.doOnce(ctx, method, path, body, headers, out, wantStatus)

		if lastErr == nil || !isRetryable(lastErr) {
			return lastErr
		}
	}

	return fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *RestClient) doOnce(ctx context.Context, method, path string, body []byte, headers map[string]string, out any, wantStatus int) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewBuffer(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, path, reader)
	if err != nil {
		return err
	}
	c.auth(req)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	tflog.Debug(ctx, "sending request", map[string]any{
		"method": method,
		"path":   path,
	})

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tflog.Debug(ctx, "received response", map[string]any{
		"status": resp.StatusCode,
	})

	switch {
	case resp.StatusCode == wantStatus && out != nil:
		return json.NewDecoder(resp.Body).Decode(out)
	case resp.StatusCode == wantStatus:
		// Success without a response body (e.g. DELETE 204).
		return nil
	case resp.StatusCode == http.StatusNotFound:
		return APIErrorFor(method, path, resp.StatusCode, "not found", entity.ErrClusterNotFound)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return APIErrorFor(method, path, resp.StatusCode, "unauthorized: check api_token", nil)
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		return APIErrorFor(method, path, resp.StatusCode, resp.Status, nil)
	default:
		return APIErrorFor(method, path, resp.StatusCode, resp.Status, nil)
	}
}

func (c *RestClient) auth(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
}

func isRetryable(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.IsRetryable()
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt) * baseBackoff
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
