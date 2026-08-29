package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"terraform-provider-ai/api"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: api.NewServer(),
	}

	go func() {
		log.Println("Mock API running on :8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s", err)
		}
	}()

	<-ctx.Done()
	stop()

	log.Println("Shutting down gracefully, press Ctrl+C again to force...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Timeout/error — force shutdown.
		log.Fatalf("server forced to shutdown: %s", err)
	}

	log.Println("Server exiting")
}
