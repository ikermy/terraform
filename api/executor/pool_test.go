package executor

import (
	"context"
	"testing"
	"time"
)

func TestPool_CompletesAllTasks(t *testing.T) {
	const delay = 20 * time.Millisecond
	p := NewPool(3, delay, time.Second)
	defer p.Close()

	const n = 12
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.WaitAll(ctx); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	for i := 0; i < n; i++ {
		if s := p.Status(fmtID(i)); s != StatusCompleted {
			t.Fatalf("task %d status = %q, want completed", i, s)
		}
	}
}

func TestPool_LimitsParallelism(t *testing.T) {
	const delay = 50 * time.Millisecond
	p := NewPool(2, delay, time.Second)
	defer p.Close()

	if got := p.Workers(); got != 2 {
		t.Fatalf("expected 2 workers, got %d", got)
	}

	// Running counter must never exceed the worker count.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < 8; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if p.Running() > 2 {
			t.Fatalf("running = %d, exceeded worker count 2", p.Running())
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestPool_Resize(t *testing.T) {
	const delay = 10 * time.Millisecond
	p := NewPool(2, delay, time.Second)
	defer p.Close()

	p.Resize(4)
	waitWorkers(t, p, 4)
	p.Resize(1)
	waitWorkers(t, p, 1)
}

func TestPool_GracefulShrinkKeepsQueue(t *testing.T) {
	const delay = 30 * time.Millisecond
	p := NewPool(4, delay, time.Second)
	defer p.Close()

	const n = 20
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	// Shrink while tasks are queued/executing; no task should be lost.
	p.Resize(1)
	waitWorkers(t, p, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.WaitAll(ctx); err != nil {
		t.Fatalf("WaitAll: %v", err)
	}

	completed := 0
	for i := 0; i < n; i++ {
		if p.Status(fmtID(i)) == StatusCompleted {
			completed++
		}
	}
	if completed != n {
		t.Fatalf("expected %d completed, got %d", n, completed)
	}
}

func TestPool_Timeout(t *testing.T) {
	// timeout much shorter than delay -> tasks must time out.
	p := NewPool(1, time.Second, 30*time.Millisecond)
	defer p.Close()

	if err := p.Submit("slow"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if s := p.Status("slow"); s != StatusTimedOut {
		t.Fatalf("expected timed_out, got %q", s)
	}
}

func fmtID(i int) string {
	return "task-" + string(rune('a'+i))
}

func waitWorkers(t *testing.T, p *Pool, want int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if p.Workers() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("workers = %d, want %d", p.Workers(), want)
}
