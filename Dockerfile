# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mock-api ./api/cmd

# Runtime stage
FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
USER app

COPY --from=builder /out/mock-api /usr/local/bin/mock-api

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/mock-api"]
