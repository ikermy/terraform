# terraform-provider-ai

A pet Terraform Provider for managing AI clusters against a mock REST API.
Built with the Terraform Plugin Framework (protocol v6) and clean architecture
(`entity` → `usecase` → `repository` → `delivery`).

## Features

- `aiprovider_cluster` resource with full CRUD against the mock API
- `aiprovider_job` resource with full CRUD against the mock API
- `terraform import` for both resources (`aiprovider_cluster.*`, `aiprovider_job.*`)
- `aiprovider_cluster` data source (read-only, lookup by id)
- Batch (concurrent) cluster creation via `ClusterInteractor.BatchCreateClusters`
- Dynamic worker pool (`api/executor`) with runtime resize, per-task timeout
  and graceful shutdown (emulated job execution)
- Transport-agnostic repositories: HTTP/REST and gRPC clients behind the same interfaces
- Mock API with both HTTP (`/clusters`, `/jobs`) and gRPC servers
- Graceful shutdown for the mock server
- Diff suppression for `model` (case-insensitive), plan modifiers
- HTTP retries with backoff, classified errors (`APIError`), domain sentinels
- CI/CD via GitHub Actions and releases via goreleaser

## Project layout

```
├── main.go                     # provider binary entrypoint (providerserver.Serve)
├── api/                        # mock REST server (package api) + runner
├── examples/main.tf            # usage examples
├── docs/                       # generated documentation (tfplugindocs)
├── internal/
│   ├── entity/                 # domain models + sentinel errors
│   ├── usecase/                # business rules
│   ├── repository/             # REST client, APIError, mapping to domain
│   └── delivery/               # provider, resource, data source
```

## Requirements

- [Go](https://go.dev/) 1.22+
- [Terraform](https://www.terraform.io/) 1.5+

## Usage

### 1. Start the mock API

HTTP (default):

```bash
go run ./api/cmd
```

gRPC (alternative transport):

```bash
go run ./api/grpc/cmd
```

The HTTP mock listens on `http://localhost:8080`, the gRPC mock on `:9090`.

### 2. Build the provider

```bash
go build -ldflags "-X main.version=0.1.0" -o terraform-provider-ai .
```

### 3. Install the provider locally

Terraform looks up provider binaries under your home `.terraform.d/plugins`:

```bash
mkdir -p ~/.terraform.d/plugins/registry.example.com/ai/ai/0.1.0/windows_amd64
cp terraform-provider-ai.exe ~/.terraform.d/plugins/registry.example.com/ai/ai/0.1.0/windows_amd64/
```

Or use `make install`.

### 4. Configure and apply

```hcl
terraform {
  required_providers {
    aiprovider = {
      source  = "registry.example.com/ai/ai"
      version = "0.1.0"
    }
  }
}

provider "aiprovider" {
  endpoint  = "http://localhost:8080"
  api_token = "test-token"   # optional; falls back to AIPROVIDER_API_TOKEN
  transport = "rest"         # "rest" or "grpc"; grpc endpoint is a host:port target
}

resource "aiprovider_cluster" "demo" {
  name     = "demo-cluster"
  replicas = 3
  model    = "gpt-mini"
}

resource "aiprovider_job" "demo" {
  name     = "demo-job"
  command  = "echo hello"
  priority = 5
}

data "aiprovider_cluster" "by_id" {
  id = aiprovider_cluster.demo.id
}
```

```bash
cd examples
terraform init
terraform apply
```

## Import

```bash
terraform import aiprovider_cluster.demo <cluster-id>
```

## Development

```bash
make test        # unit tests
make test-acc    # acceptance tests (TF_ACC=1)
make lint        # golangci-lint
make vet         # go vet
make fmt         # gofmt
make docs        # regenerate docs via tfplugindocs
make coverage    # test coverage report
make release     # goreleaser release (needs a tag)
```

## Configuration reference

| Argument | Type | Description |
|----------|------|-------------|
| `endpoint` | string | Base URL of the AI API. Defaults to `http://localhost:8080`. For `transport = "grpc"` use a host:port target. |
| `api_token` | string | Optional bearer token. If unset, read from `AIPROVIDER_API_TOKEN`. |
| `transport` | string | `"rest"` (default) or `"grpc"`. |

## License

MIT
