.PHONY: build run test check test-acc proto docs install docker-build docker-build-ci docker-tf compose-up release

BINARY   := terraform-provider-ai
VERSION  ?= 0.1.0
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOOS     := $(shell go env GOOS)
GOARCH   := $(shell go env GOARCH)
EXE      := $(if $(filter windows,$(GOOS)),.exe,)
REGISTRY := registry.example.com/ai/ai
IMAGE    := ai-dev

# --- Local Go ---
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)$(EXE) ./cmd/provider

run:
	go run ./cmd/api

test:
	go test ./... -count=1

# Lint + race tests; run before committing.
check:
	golangci-lint run ./...
	go test -race -count=1 ./...

test-acc:
	TF_ACC=1 go test ./... -v -count=1

# Regenerate protobuf/gRPC code. Versions are pinned so CI and developers
# produce byte-identical output (no spurious diffs).
PROTO_GO_VER     := v1.36.11
PROTO_GO_GRPC_VER := v1.6.2
proto:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTO_GO_VER)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTO_GO_GRPC_VER)
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/ai/v1/ai.proto

docs:
	go generate ./internal/delivery

install: build
	$(eval PLUGIN_DIR := $(HOME)/.terraform.d/plugins/$(REGISTRY)/$(VERSION)/$(GOOS)_$(GOARCH))
	mkdir -p $(PLUGIN_DIR)
	cp bin/$(BINARY)$(EXE) $(PLUGIN_DIR)/$(BINARY)$(EXE)

# --- Docker ---
# Build the dev image (Terraform + provider installed); Dockerfile target "dev".
docker-build:
	docker build --target dev -f Dockerfile -t $(IMAGE) .

# Build both targets in CI, persisting the BuildKit cache to GitHub Actions so
# repeat runs are fast (requires docker buildx).
docker-build-ci:
	docker buildx build --target api --cache-from type=gha --cache-to type=gha,mode=max -t ai-mock .
	docker buildx build --target dev --cache-from type=gha --cache-to type=gha,mode=max -t ai-dev .

# Run any terraform command in the dev image, e.g.:
#   make docker-tf CMD=validate
#   make docker-tf CMD=plan
#   make docker-tf CMD="apply -auto-approve"
docker-tf:
	docker run --rm --entrypoint sh -v $(CURDIR)/examples:/work $(IMAGE) -c \
		"terraform init -input=false && terraform $(or $(CMD),plan) -input=false"

# Mock API server via docker compose (up / logs / down).
compose-up:
	docker compose up -d --build

release:
	goreleaser release --clean
