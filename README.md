# terraform-provider-ai

A pet Terraform Provider for managing AI clusters/jobs against a mock API.
Built with the Terraform Plugin Framework (protocol v6) and clean architecture
(`entity` → `usecase` → `repository` → `delivery`).

## Features

- `aiprovider_cluster` resource with full CRUD against the mock API
- `aiprovider_job` resource with full CRUD against the mock API
- `terraform import` for both resources (`aiprovider_cluster.*`, `aiprovider_job.*`)
- `aiprovider_cluster` data source (read-only, lookup by id)
- Batch (concurrent) cluster creation via `ClusterInteractor.BatchCreateClusters`
- Dynamic worker pool (`api/executor`) with runtime resize, per-task timeout,
  graceful shutdown and configurable buffers/status retention
- **Jobs are actually executed**: each job's `command` runs as a real subprocess
  in the worker pool (bounded by a per-job timeout)
- Transport-agnostic repositories: HTTP/REST and gRPC clients behind the same interfaces
- Mock API with both HTTP (`/clusters`, `/jobs`) and gRPC servers
- Graceful shutdown for the mock server
- Diff suppression for `model` (case-insensitive), plan modifiers
- HTTP retries with backoff, classified errors (`APIError`), domain sentinels
- CI/CD via GitHub Actions and releases via goreleaser
- Docker-first development: everything runs in containers

## Project layout

```
├── cmd/
│   ├── provider/main.go     # Terraform Provider binary (providerserver.Serve)
│   └── api/main.go          # Mock API binary (HTTP + gRPC)
├── api/
│   ├── http/                # HTTP mock server (package http) + tests
│   ├── grpc/                # gRPC mock server (package grpc) + tests
│   └── executor/            # worker pool (package executor) + tests
├── proto/ai/v1/             # protobuf definitions + generated code
├── examples/main.tf         # usage examples
├── docs/                    # generated documentation (tfplugindocs)
├── internal/
│   ├── entity/              # domain models + sentinel errors
│   ├── usecase/             # business rules
│   ├── repository/          # REST + gRPC clients, APIError, mapping to domain
│   └── delivery/            # provider, resource, data source
├── Dockerfile               # multi-stage: target `api` (mock) / target `dev` (Terraform)
├── docker-compose.yml       # mock API (HTTP :8080, gRPC :9090)
├── Makefile                 # all commands (Go + Docker)
├── dev.ps1                  # Windows PowerShell wrappers (no make needed)
├── .goreleaser.yml          # release build config
├── .golangci.yml            # linter config
└── .github/workflows/ci.yml
```

## Requirements

- [Docker](https://www.docker.com/) (recommended; everything runs in containers)
- [Go](https://go.dev/) 1.26+ (only if you run outside Docker)
- [Terraform](https://www.terraform.io/) 1.5+ (via the Docker dev image)

## Docker-first workflow

All commands are centralized in the `Makefile` (and mirrored in `dev.ps1` for
Windows PowerShell where `make` is unavailable).

### Run the mock API (HTTP + gRPC) via Docker Compose

```bash
make compose-up        # build + start mock (HTTP :8080, gRPC :9090)
docker compose logs -f # follow logs
docker compose down    # stop/remove
```

```powershell
# Windows PowerShell
. ./dev.ps1
Invoke-MockUp      # docker compose up -d --build
Invoke-MockLogs    # docker compose logs -f
Invoke-MockDown    # docker compose down
```

Once running, poke the endpoints:

```powershell
# HTTP
Invoke-RestMethod http://localhost:8080/clusters
Invoke-RestMethod http://localhost:8080/jobs -Method Post -ContentType "application/json" `
  -Body '{"name":"j","command":"echo hello","priority":1}'

# gRPC
grpcurl -plaintext -d '{"cluster":{"name":"demo","replicas":2}}' localhost:9090 ai.v1.ClusterService/CreateCluster
```

### Validate, plan and apply the Terraform configuration

```bash
make docker-build              # build dev image (Terraform + provider installed)
make docker-tf                 # init + plan (default)
make docker-tf CMD=validate    # init + validate
make docker-tf CMD=plan        # init + plan
make docker-tf CMD="apply -auto-approve"   # init + apply
```

```powershell
. ./dev.ps1
Invoke-DevBuild
Invoke-DevValidate   # Success! The configuration is valid.
Invoke-DevPlan
Invoke-DevTf 'apply -auto-approve'
```

Run the mock API first (`make compose-up`), then apply. The provider connects to
`http://localhost:8080` (the mock API exposed from the `ai-mock` container).

## Configuration (`examples/main.tf`)

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

## Import

```bash
terraform import aiprovider_cluster.demo <cluster-id>
```

## Development (outside Docker)

```bash
make build      # build the provider binary (./cmd/provider)
make run        # start mock API locally (go run ./cmd/api)
make test       # unit tests
make check      # golangci-lint + go test -race (run before commit)
make test-acc   # acceptance tests (TF_ACC=1)
make proto      # regenerate protobuf/gRPC code
make docs       # regenerate docs via tfplugindocs
make install    # install provider into the filesystem mirror
make release    # goreleaser release (needs a tag)
```

## Configuration reference

| Argument | Type | Description |
|----------|------|-------------|
| `endpoint` | string | Base URL of the AI API. Defaults to `http://localhost:8080`. For `transport = "grpc"` use a host:port target. |
| `api_token` | string | Optional bearer token. If unset, read from `AIPROVIDER_API_TOKEN`. |
| `transport` | string | `"rest"` (default) or `"grpc"`. |

## License

MIT
