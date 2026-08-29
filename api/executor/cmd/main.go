package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"terraform-provider-ai/api/executor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	p := executor.NewPool(2, 300*time.Millisecond, 2*time.Second,
		executor.WithQueueSize(64),
		executor.WithResultBuffer(64),
		executor.WithMaxStatus(1000),
	)
	defer p.Close()

	// Submit a batch of jobs and resize the pool while they run.
	for i := 0; i < 10; i++ {
		if err := p.Submit(fmt.Sprintf("job-%d", i)); err != nil {
			fmt.Println("submit:", err)
			return
		}
	}

	go func() {
		time.Sleep(700 * time.Millisecond)
		fmt.Println("resizing to 4 workers")
		p.Resize(4)
	}()

	fmt.Println("executor running; press Ctrl+C to shut down gracefully")
	<-ctx.Done()
	fmt.Println("shutting down...")
}
