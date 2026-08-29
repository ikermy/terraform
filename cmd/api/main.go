package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	mockgrpc "terraform-provider-ai/api/grpc"
	mockhttp "terraform-provider-ai/api/http"
	"terraform-provider-ai/config"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

// RunConfig configures the combined mock API server.
type RunConfig struct {
	HTTPAddr   string
	GRPCAddr   string
	Workers    int
	JobTimeout time.Duration
}

func (c RunConfig) withDefaults() RunConfig {
	if c.HTTPAddr == "" {
		c.HTTPAddr = config.DefaultHTTPAddr
	}
	if c.GRPCAddr == "" {
		c.GRPCAddr = config.DefaultGRPCAddr
	}
	if c.Workers < 1 {
		c.Workers = config.DefaultWorkers
	}
	if c.JobTimeout == 0 {
		c.JobTimeout = config.DefaultJobTimeout
	}
	return c
}

// run starts both the HTTP and gRPC mock servers and blocks until ctx is
// cancelled, then shuts both down gracefully.
func run(ctx context.Context, cfg RunConfig) error {
	cfg = cfg.withDefaults()

	httpSrv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: mockhttp.NewServer(
			mockhttp.WithWorkers(cfg.Workers),
			mockhttp.WithJobTimeout(cfg.JobTimeout),
		),
	}

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return err
	}
	grpcSrv := grpc.NewServer()
	mock := mockgrpc.NewServer(
		mockgrpc.WithWorkers(cfg.Workers),
		mockgrpc.WithJobTimeout(cfg.JobTimeout),
	)
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
		grpcSrv.Stop()
		_ = httpSrv.Close()
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// GracefulStop blocks until all active gRPC calls finish, which can exceed
	// the timeout if clients hold long connections. Run it in a goroutine and
	// force Stop() once the deadline expires.
	stopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcSrv.Stop() // force-close gRPC after the timeout
	}

	return httpSrv.Shutdown(shutdownCtx)
}

func main() {
	var (
		httpAddr   = flag.String("http-addr", config.DefaultHTTPAddr, "HTTP address")
		grpcAddr   = flag.String("grpc-addr", config.DefaultGRPCAddr, "gRPC address")
		workers    = flag.Int("workers", config.DefaultWorkers, "number of worker pool workers")
		jobTimeout = flag.Duration("job-timeout", config.DefaultJobTimeout, "per-job deadline")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg := RunConfig{
		HTTPAddr:   *httpAddr,
		GRPCAddr:   *grpcAddr,
		Workers:    *workers,
		JobTimeout: *jobTimeout,
	}

	log.Println("starting mock API (HTTP + gRPC); press Ctrl+C to shut down")
	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
	log.Println("mock API shut down gracefully")
}
