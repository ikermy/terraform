.PHONY: build test test-race test-acc vet lint fmt tidy gen gen-proto docs coverage install release

BINARY   := terraform-provider-ai
VERSION  ?= 0.1.0
LDFLAGS  := -s -w -X main.version=$(VERSION)
PKG_LIST := ./...

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

test:
	go test $(PKG_LIST) -count=1

test-race:
	go test $(PKG_LIST) -race -count=1

test-acc:
	TF_ACC=1 go test $(PKG_LIST) -count=1 -v

vet:
	go vet $(PKG_LIST)

lint:
	golangci-lint run $(PKG_LIST)

fmt:
	gofmt -w .

tidy:
	go mod tidy

gen:
	go generate ./...

gen-proto:
	protoc --go_out=. --go_opt=module=terraform-provider-ai \
		--go-grpc_out=. --go-grpc_opt=module=terraform-provider-ai \
		proto/ai/v1/ai.proto

docs:
	go generate ./internal/delivery

coverage:
	go test $(PKG_LIST) -count=1 -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Local install for terraform init:
# $(HOME)/.terraform.d/plugins/registry.example.com/ai/ai/$(VERSION)/windows_amd64/terraform-provider-ai.exe
install: build
	mkdir -p ~/.terraform.d/plugins/registry.example.com/ai/ai/$(VERSION)/windows_amd64
	cp bin/$(BINARY).exe ~/.terraform.d/plugins/registry.example.com/ai/ai/$(VERSION)/windows_amd64/terraform-provider-ai.exe

release:
	goreleaser release --clean
