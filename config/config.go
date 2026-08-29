// Package config holds shared constants and configuration defaults used across
// all layers (internal, api, cmd). It is a neutral package: it imports no
// project-internal package, so any layer may depend on it without violating
// the dependency rules.
package config

import "time"

// ─── Batch operations ─────────────────────────────────────────────────────────

const (
	// BatchConcurrencyLimit bounds how many clusters are created in parallel
	// against the API. An execution concern, not a domain rule.
	BatchConcurrencyLimit = 10
)

// ─── HTTP client (repository) ─────────────────────────────────────────────────

const (
	// RequestTimeout is the default per-request HTTP timeout.
	RequestTimeout = 30 * time.Second
	// MaxRetries is the number of retries for transient (5xx/429) responses.
	MaxRetries = 3
	// BaseBackoff is the initial retry backoff; it grows linearly per attempt.
	BaseBackoff = 200 * time.Millisecond
)

// ─── Worker pool (api/executor) defaults ──────────────────────────────────────

const (
	// DefaultQueueSize is the default buffered size of the task channel.
	DefaultQueueSize = 128
	// DefaultResultBuffer is the default buffered size of the results channel.
	DefaultResultBuffer = 256
	// DefaultMaxStatus bounds the number of retained status entries.
	DefaultMaxStatus = 10_000
)

// ─── Mock API server defaults ─────────────────────────────────────────────────

const (
	// DefaultHTTPAddr is the default HTTP listen address.
	DefaultHTTPAddr = ":8080"
	// DefaultGRPCAddr is the default gRPC listen address.
	DefaultGRPCAddr = ":9090"
	// DefaultWorkers is the default number of worker-pool workers.
	DefaultWorkers = 2
	// DefaultJobTimeout is the default per-job deadline.
	DefaultJobTimeout = 2 * time.Second
)

// ─── Provider defaults ────────────────────────────────────────────────────────

const (
	// DefaultEndpoint is the default API endpoint the provider talks to.
	DefaultEndpoint = "http://localhost:8080"
	// EnvAPIToken is the environment variable the api_token falls back to.
	EnvAPIToken = "AIPROVIDER_API_TOKEN"
)
