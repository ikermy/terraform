package api

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	mockgrpc "terraform-provider-ai/api/grpc"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

// RunConfig configures the combined mock API server.
type RunConfig struct {
	HTTPAddr   string
	GRPCAddr   string
	Workers    int
	JobDelay   time.Duration
	JobTimeout time.Duration
}

// withDefaults fills in sensible defaults for unset fields.
func (c RunConfig) withDefaults() RunConfig {
	if c.HTTPAddr == "" {
		c.HTTPAddr = ":8080"
	}
	if c.GRPCAddr == "" {
		c.GRPCAddr = ":9090"
	}
	if c.Workers < 1 {
		c.Workers = 2
	}
	if c.JobDelay == 0 {
		c.JobDelay = 300 * time.Millisecond
	}
	if c.JobTimeout == 0 {
		c.JobTimeout = 2 * time.Second
	}
	return c
}

// Run starts both the HTTP and gRPC mock servers and blocks until ctx is
// cancelled, then shuts both down gracefully. Any fatal server error is
// returned.
func Run(ctx context.Context, cfg RunConfig) error {
	cfg = cfg.withDefaults()

	// HTTP server, configured with the worker pool options.
	httpSrv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: NewServer(
			WithWorkers(cfg.Workers),
			WithJobDelay(cfg.JobDelay),
			WithJobTimeout(cfg.JobTimeout),
		),
	}

	// gRPC server.
	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	grpcSrv := grpc.NewServer()
	mock := mockgrpc.NewServer()
	aiv1.RegisterClusterServiceServer(grpcSrv, mock)
	aiv1.RegisterJobServiceServer(grpcSrv, mock)

	errCh := make(chan error, 2)
	go func() {
		log.Printf("HTTP mock API listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		log.Printf("gRPC mock API listening on %s", cfg.GRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		// Graceful shutdown requested (signal).
	case err := <-errCh:
		// One of the servers failed fatally.
		grpcSrv.Stop()
		_ = httpSrv.Close()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	grpcSrv.GracefulStop()
	return httpSrv.Shutdown(shutdownCtx)
}
