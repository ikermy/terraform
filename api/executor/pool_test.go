package executor

import (
	"context"
	"errors"
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

func TestPool_CloseDrainsQueue(t *testing.T) {
	p := NewPool(2, 10*time.Millisecond, time.Second)

	const n = 10
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	// Close must drain the whole queue, not drop pending tasks.
	p.Close()
	for i := 0; i < n; i++ {
		if s := p.Status(fmtID(i)); s != StatusCompleted {
			t.Fatalf("task %d status = %q after Close, want completed", i, s)
		}
	}
}

func TestPool_Results(t *testing.T) {
	p := NewPool(2, 5*time.Millisecond, time.Second)

	const n = 5
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	seen := make(map[string]TaskStatus)
	deadline := time.After(2 * time.Second)
	for len(seen) < n {
		select {
		case r := <-p.Results():
			seen[r.ID] = r.Status
		case <-deadline:
			t.Fatalf("only received %d results, want %d", len(seen), n)
		}
	}
	for _, s := range seen {
		if s != StatusCompleted {
			t.Fatalf("unexpected result status %q", s)
		}
	}
	p.Close()
}

func TestPool_StatusEviction(t *testing.T) {
	p := NewPool(1, time.Millisecond, time.Second)
	defer p.Close()
	p.maxStatus = 2 // force eviction for the test

	const n = 8
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i)); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	p.WaitAll(context.Background())

	p.mu.Lock()
	size := len(p.status)
	tq := len(p.terminal)
	p.mu.Unlock()
	if size > 2 {
		t.Fatalf("expected status map bounded by 2, got %d entries", size)
	}
	if tq > 2 {
		t.Fatalf("expected terminal queue bounded by 2, got %d entries", tq)
	}
}

func TestPool_Forget(t *testing.T) {
	p := NewPool(1, time.Millisecond, time.Second)
	defer p.Close()

	if err := p.Submit("x"); err != nil {
		t.Fatalf("submit: %v", err)
	}
	p.WaitAll(context.Background())
	if p.Status("x") != StatusCompleted {
		t.Fatalf("expected completed, got %q", p.Status("x"))
	}
	p.Forget("x")
	if p.Status("x") != "" {
		t.Fatalf("expected empty status after Forget, got %q", p.Status("x"))
	}
}

func TestPool_SubmitAfterClose(t *testing.T) {
	p := NewPool(1, time.Millisecond, time.Second)
	p.Close()
	if err := p.Submit("x"); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
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
