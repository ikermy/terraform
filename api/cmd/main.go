package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"terraform-provider-ai/api"
)

func main() {
	var (
		httpAddr   = flag.String("http-addr", ":8080", "HTTP address")
		grpcAddr   = flag.String("grpc-addr", ":9090", "gRPC address")
		workers    = flag.Int("workers", 2, "number of worker pool workers")
		jobDelay   = flag.Duration("job-delay", 300*time.Millisecond, "emulated job execution time")
		jobTimeout = flag.Duration("job-timeout", 2*time.Second, "per-job deadline")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	cfg := api.RunConfig{
		HTTPAddr:   *httpAddr,
		GRPCAddr:   *grpcAddr,
		Workers:    *workers,
		JobDelay:   *jobDelay,
		JobTimeout: *jobTimeout,
	}

	log.Println("starting mock API (HTTP + gRPC); press Ctrl+C to shut down")
	if err := api.Run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
	log.Println("mock API shut down gracefully")
}
