# syntax=docker/dockerfile:1

# ---- Base builder: static Go build environment (Debian image includes make,
# needed by dev-build's `make build install`; the builder does not ship in the
# final api image). Version pinned to go.mod (go 1.26.4). ----
FROM golang:1.26.4 AS builder
ENV CGO_ENABLED=0
WORKDIR /src

# Download module deps first, with BuildKit cache so repeat builds are fast.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# ---- api-build: compile the mock API binary ----
FROM builder AS api-build
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/mock-api ./cmd/api

# ---- api: minimal distroless image (no shell, non-root, static binary) ----
FROM gcr.io/distroless/static-debian12:nonroot AS api
WORKDIR /
COPY --from=api-build /out/mock-api /usr/local/bin/mock-api
EXPOSE 8080 9090
ENTRYPOINT ["/usr/local/bin/mock-api"]

# ---- dev-build: build and install the Terraform provider ----
FROM builder AS dev-build
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    make build install

# ---- dev: Terraform CLI + provider installed into the filesystem mirror ----
FROM hashicorp/terraform:1.6 AS dev
COPY --from=dev-build /root/.terraform.d/plugins/ /root/.terraform.d/plugins/
WORKDIR /work
