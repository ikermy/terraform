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
// execution for `delay`; if `timeout` is shorter than `delay` the task is
// marked timed_out (deterministic: delay <= timeout means completed, otherwise
// timed_out). Shrinking closes the stop channel of the excess workers: they
// finish the task they are currently executing before exiting, so queued
// tasks are never dropped. Close drains the remaining queue before shutdown.
type Pool struct {
	tasks   chan string
	resize  chan struct{} // wake-up signal (cap 1); latest target read from target
	stop    chan struct{}
	done    chan struct{} // closed when the coordinator exits
	results chan Result
	wg      sync.WaitGroup
	delay   time.Duration
	timeout time.Duration

	mu     sync.Mutex
	status map[string]TaskStatus

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
			// Drain: let the workers finish every queued task before closing.
			for p.running.Load() > 0 || len(p.tasks) > 0 {
				time.Sleep(5 * time.Millisecond)
			}
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
		case <-stop:
			return
		}
	}
}

// execute emulates work for a single task using a single timer. The outcome
// is deterministic: completed if delay <= timeout, timed_out otherwise.
func (p *Pool) execute(id string) {
	p.setStatus(id, StatusRunning)
	p.running.Add(1)
	defer p.running.Add(-1)

	effective := p.delay
	final := StatusCompleted
	if p.timeout < p.delay {
		effective = p.timeout
		final = StatusTimedOut
	}

	timer := time.NewTimer(effective)
	defer timer.Stop()
	<-timer.C

	p.setStatus(id, final)
	p.emitResult(Result{ID: id, Status: final})
}

// Submit enqueues a task. It returns ErrClosed if the pool is shutting down.
func (p *Pool) Submit(id string) error {
	if p.closed.Load() {
		return ErrClosed
	}
	p.setStatus(id, StatusQueued)
	select {
	case p.tasks <- id:
		return nil
	case <-p.stop:
		return ErrClosed
	}
}

// Resize changes the number of workers at runtime. It is non-blocking: the
// latest target is stored atomically and applied by the coordinator.
func (p *Pool) Resize(n int) {
	if n < 0 {
		n = 0
	}
	p.target.Store(int64(n))
	select {
	case p.resize <- struct{}{}:
	default: // coordinator will read the latest target on the next signal
	}
}

// Status returns the current lifecycle status of a task.
func (p *Pool) Status(id string) TaskStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status[id]
}

// Workers returns the current number of workers.
func (p *Pool) Workers() int64 {
	return p.max.Load()
}

// Running returns the number of tasks currently executing.
func (p *Pool) Running() int64 {
	return p.running.Load()
}

// Results returns a read-only channel of completed task results. It is
// non-blocking and drops results if the buffer is full (no subscriber).
func (p *Pool) Results() <-chan Result {
	return p.results
}

// Close shuts the pool down gracefully: it stops accepting new tasks, waits
// for all queued tasks to finish (no drops), then terminates the workers.
func (p *Pool) Close() {
	if p.closed.Swap(true) {
		return
	}
	close(p.stop)
	<-p.done
	p.wg.Wait()
	close(p.results)
}

func (p *Pool) setStatus(id string, s TaskStatus) {
	p.mu.Lock()
	p.status[id] = s
	p.mu.Unlock()
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
