package executor

import (
	"context"
	"testing"
	"time"
)

func quickWork(_ context.Context, _ Task) error { return nil }

func blockWork(ctx context.Context, _ Task) error {
	<-ctx.Done()
	return ctx.Err()
}

func slowWork(ctx context.Context, _ Task) error {
	select {
	case <-time.After(50 * time.Millisecond):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPool_CompletesAllTasks(t *testing.T) {
	p := NewPool(3, quickWork, time.Second)
	defer p.Close()

	const n = 12
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
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
	p := NewPool(2, slowWork, time.Second)
	defer p.Close()

	if got := p.Workers(); got != 2 {
		t.Fatalf("expected 2 workers, got %d", got)
	}

	for i := 0; i < 8; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if p.Running() > 2 {
			t.Fatalf("running = %d, exceeded worker count 2", p.Running())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPool_Resize(t *testing.T) {
	p := NewPool(2, quickWork, time.Second)
	defer p.Close()

	p.Resize(4)
	waitWorkers(t, p, 4)
	p.Resize(1)
	waitWorkers(t, p, 1)
}

func TestPool_GracefulShrinkKeepsQueue(t *testing.T) {
	p := NewPool(4, quickWork, time.Second)
	defer p.Close()

	const n = 20
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
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
	p := NewPool(1, blockWork, 30*time.Millisecond)
	defer p.Close()

	if err := p.Submit("slow", "echo ok"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if s := p.Status("slow"); s != StatusTimedOut {
		t.Fatalf("expected timed_out, got %q", s)
	}
}

func TestPool_CloseDrainsQueue(t *testing.T) {
	p := NewPool(2, quickWork, time.Second)

	const n = 10
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	p.Close()
	for i := 0; i < n; i++ {
		if s := p.Status(fmtID(i)); s != StatusCompleted {
			t.Fatalf("task %d status = %q after Close, want completed", i, s)
		}
	}
}

func TestPool_Results(t *testing.T) {
	p := NewPool(2, quickWork, time.Second)

	const n = 5
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
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

func TestPool_Options(t *testing.T) {
	p := NewPool(1, quickWork, time.Second,
		WithQueueSize(16),
		WithResultBuffer(8),
		WithMaxStatus(3),
	)
	defer p.Close()

	if got := cap(p.tasks); got != 16 {
		t.Fatalf("queue size = %d, want 16", got)
	}
	if got := cap(p.results); got != 8 {
		t.Fatalf("result buffer = %d, want 8", got)
	}
	if got := p.maxStatus; got != 3 {
		t.Fatalf("maxStatus = %d, want 3", got)
	}

	for i := 0; i < 10; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	p.WaitAll(context.Background())

	p.mu.Lock()
	size := len(p.status)
	p.mu.Unlock()
	if size > 3 {
		t.Fatalf("expected status bounded by 3 (via option), got %d", size)
	}
}

func TestPool_StatusEviction(t *testing.T) {
	p := NewPool(1, quickWork, time.Second)
	defer p.Close()
	p.maxStatus = 2 // force eviction for the test

	const n = 8
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	p.WaitAll(context.Background())

	p.mu.Lock()
	size := len(p.status)
	tq := p.terminal.Len()
	p.mu.Unlock()
	if size > 2 {
		t.Fatalf("expected status map bounded by 2, got %d entries", size)
	}
	if tq > 2 {
		t.Fatalf("expected terminal queue bounded by 2, got %d entries", tq)
	}
}

func TestPool_Forget(t *testing.T) {
	p := NewPool(1, quickWork, time.Second)
	defer p.Close()

	if err := p.Submit("x", "echo ok"); err != nil {
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

func TestPool_CloseAfterResizeZero(t *testing.T) {
	p := NewPool(2, quickWork, time.Second)
	p.Resize(0)

	const n = 5
	for i := 0; i < n; i++ {
		if err := p.Submit(fmtID(i), "echo ok"); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	done := make(chan struct{})
	go func() {
		p.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close deadlocked after Resize(0)")
	}

	for i := 0; i < n; i++ {
		if s := p.Status(fmtID(i)); s != StatusCompleted {
			t.Fatalf("task %d status = %q, want completed", i, s)
		}
	}
}

func TestPool_WaitAllContextCancellation(t *testing.T) {
	p := NewPool(1, blockWork, 2*time.Second)
	defer p.Close()

	if err := p.Submit("slow", "echo ok"); err != nil {
		t.Fatalf("submit: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := p.WaitAll(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("WaitAll did not return promptly on cancelled ctx: %v", time.Since(start))
	}
}

func TestPool_SubmitAfterClose(t *testing.T) {
	p := NewPool(1, quickWork, time.Second)
	p.Close()
	if err := p.Submit("x", "echo ok"); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
	if s := p.Status("x"); s != "" {
		t.Fatalf("expected no orphan status after failed submit, got %q", s)
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
