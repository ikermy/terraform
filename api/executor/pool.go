// Package executor provides a dynamic worker pool that emulates job
// execution with a configurable delay and per-task timeout. Workers can be
// resized at runtime without dropping queued tasks.
package executor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrClosed is returned by Submit when the pool is shutting down.
var ErrClosed = errors.New("executor: pool closed")

// TaskStatus describes the lifecycle of a submitted job.
type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusTimedOut  TaskStatus = "timed_out"
)

// Result reports the final outcome of a task.
type Result struct {
	ID     string
	Status TaskStatus
}

// Pool is a dynamic worker pool.
//
// A single coordinator goroutine owns the worker set and reacts to resize
// signals. Each worker consumes tasks from a buffered channel and emulates
// execution for `delay`, bounded by a per-task `timeout` via context
// cancellation (the first of the two to elapse decides the outcome). Shrinking
// closes the stop channel of the excess workers: they finish the task they are
// currently executing before exiting, so queued tasks are never dropped. Close
// drains the remaining queue before shutdown.
type Pool struct {
	tasks   chan string
	resize  chan struct{} // wake-up signal (cap 1); latest target read from target
	stop    chan struct{}
	done    chan struct{} // closed when the coordinator exits
	results chan Result
	wg      sync.WaitGroup
	delay   time.Duration
	timeout time.Duration

	mu        sync.Mutex
	cond      *sync.Cond
	pending   int // queued + in-flight tasks; guarded by mu
	status    map[string]TaskStatus
	order     []string // insertion order of status keys, for eviction
	maxStatus int      // upper bound on retained status entries

	running atomic.Int64
	max     atomic.Int64
	target  atomic.Int64
	closed  atomic.Bool
}

// NewPool starts a pool with the given number of workers. delay is the
// emulated execution time per task; timeout is the per-task deadline.
func NewPool(workers int, delay, timeout time.Duration) *Pool {
	if workers < 1 {
		workers = 1
	}
	p := &Pool{
		tasks:   make(chan string, 128),
		resize:  make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		results: make(chan Result, 256),
		delay:   delay,
		timeout: timeout,
		status:  make(map[string]TaskStatus),
	}
	p.cond = sync.NewCond(&p.mu)
	p.maxStatus = 10000
	p.target.Store(int64(workers))
	p.max.Store(int64(workers))
	go p.coordinator(workers)
	return p
}

func (p *Pool) coordinator(initial int) {
	workers := make(map[int]chan struct{})
	next := 0

	spawn := func(n int) {
		for len(workers) < n {
			stopCh := make(chan struct{})
			workers[next] = stopCh
			next++
			p.wg.Add(1)
			go p.worker(stopCh)
		}
	}

	apply := func() {
		n := int(p.target.Load())
		if n < 0 {
			n = 0
		}
		spawn(n)
		for id, ch := range workers {
			if len(workers) <= n {
				break
			}
			close(ch)
			delete(workers, id)
		}
		p.max.Store(int64(len(workers)))
	}

	spawn(initial)

	for {
		select {
		case <-p.resize:
			apply()
		case <-p.stop:
			// Drain: block until no task is pending, then stop the workers.
			// Uses sync.Cond instead of a busy-wait loop.
			p.mu.Lock()
			for p.pending > 0 {
				p.cond.Wait()
			}
			p.mu.Unlock()
			for _, ch := range workers {
				close(ch)
			}
			close(p.tasks)
			close(p.done)
			return
		}
	}
}

func (p *Pool) worker(stop chan struct{}) {
	defer p.wg.Done()
	for {
		select {
		case id, ok := <-p.tasks:
			if !ok {
				return
			}
			p.execute(id)
			p.taskDone()
		case <-stop:
			return
		}
	}
}

// execute emulates work for a single task. The task runs for `delay` within a
// context bounded by `timeout`: whichever elapses first decides the outcome.
// A single timer is used and stopped, so nothing leaks.
func (p *Pool) execute(id string) {
	p.setStatus(id, StatusRunning)
	p.running.Add(1)
	defer p.running.Add(-1)

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	timer := time.NewTimer(p.delay)
	defer timer.Stop()

	var final TaskStatus
	select {
	case <-timer.C:
		final = StatusCompleted
	case <-ctx.Done():
		final = StatusTimedOut
	}

	p.setStatus(id, final)
	p.emitResult(Result{ID: id, Status: final})
}

// Submit enqueues a task. It returns ErrClosed if the pool is shutting down.
func (p *Pool) Submit(id string) error {
	p.mu.Lock()
	if p.closed.Load() {
		p.mu.Unlock()
		return ErrClosed
	}
	p.pending++
	p.mu.Unlock()

	p.setStatus(id, StatusQueued)
	select {
	case p.tasks <- id:
		return nil
	case <-p.stop:
		p.taskDone() // roll back the pending count: never enqueued
		return ErrClosed
	}
}

// Resize changes the number of workers at runtime. It is non-blocking. The
// target is stored atomically; if several Resize calls arrive quickly, the
// coordinator applies the latest target (intermediate values may be skipped).
//
// Resize(0) closes all workers; submitted tasks stay queued in the channel and
// resume as soon as the pool is resized back to a positive number.
func (p *Pool) Resize(n int) {
	if n < 0 {
		n = 0
	}
	p.target.Store(int64(n))
	select {
	case p.resize <- struct{}{}:
	default: // a signal is already pending; the latest target will be applied
	}
}

// Status returns the current lifecycle status of a task. Terminal entries are
// auto-evicted (oldest first) once the map grows past the internal cap, so
// memory stays bounded for long-lived pools; active tasks are never evicted.
// Call Forget to drop a specific entry earlier.
func (p *Pool) Status(id string) TaskStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status[id]
}

// Forget removes a task's status entry, bounding memory for long-lived pools.
func (p *Pool) Forget(id string) {
	p.mu.Lock()
	delete(p.status, id)
	p.mu.Unlock()
}

// Workers returns the current number of workers.
func (p *Pool) Workers() int64 {
	return p.max.Load()
}

// Running returns the number of tasks currently executing.
func (p *Pool) Running() int64 {
	return p.running.Load()
}

// Results returns a read-only channel of completed task results. Delivery is
// best-effort: if the buffer is full and nobody is reading, a result may be
// dropped rather than blocking a worker.
func (p *Pool) Results() <-chan Result {
	return p.results
}

// Close shuts the pool down gracefully: it stops accepting new tasks, waits
// for all queued and in-flight tasks to finish (no drops), then terminates the
// workers.
func (p *Pool) Close() {
	if p.closed.Swap(true) {
		return
	}
	close(p.stop)
	<-p.done
	p.wg.Wait()
	close(p.results)
}

func (p *Pool) taskDone() {
	p.mu.Lock()
	p.pending--
	if p.pending == 0 {
		p.cond.Broadcast()
	}
	p.mu.Unlock()
}

func (p *Pool) setStatus(id string, s TaskStatus) {
	p.mu.Lock()
	if _, exists := p.status[id]; !exists {
		p.order = append(p.order, id)
	}
	p.status[id] = s
	if isTerminal(s) {
		p.evictLocked()
	}
	p.mu.Unlock()
}

// isTerminal reports whether a status is final and therefore a candidate for
// eviction once the map grows past maxStatus.
func isTerminal(s TaskStatus) bool {
	return s == StatusCompleted || s == StatusTimedOut
}

// evictLocked removes the oldest terminal entries until the status map fits
// within maxStatus. Active (queued/running) tasks are never evicted.
func (p *Pool) evictLocked() {
	for len(p.status) > p.maxStatus {
		evicted := false
		for i, id := range p.order {
			if isTerminal(p.status[id]) {
				delete(p.status, id)
				p.order = append(p.order[:i], p.order[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			return // all remaining entries are non-terminal
		}
	}
}

func (p *Pool) emitResult(r Result) {
	select {
	case p.results <- r:
	default: // no subscriber or full buffer; drop to avoid blocking workers
	}
}

// WaitAll blocks until all submitted tasks have reached a terminal status.
// ctx may be used to bound the wait.
func (p *Pool) WaitAll(ctx context.Context) error {
	for {
		p.mu.Lock()
		done := true
		for _, s := range p.status {
			if s == StatusQueued || s == StatusRunning {
				done = false
				break
			}
		}
		p.mu.Unlock()
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
