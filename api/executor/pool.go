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

// TaskStatus describes the lifecycle of a submitted job.
type TaskStatus string

const (
	StatusQueued    TaskStatus = "queued"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusTimedOut  TaskStatus = "timed_out"
)

// Pool is a dynamic worker pool.
//
// A single coordinator goroutine owns the worker set and reacts to resize
// requests. Each worker consumes tasks from a buffered channel and emulates
// execution for `delay`; if `timeout` elapses first the task is marked
// timed_out. Shrinking the pool closes the stop channel of the excess workers:
// they finish the task they are currently executing before exiting, so queued
// tasks are never dropped.
type Pool struct {
	tasks   chan string
	resize  chan int
	stop    chan struct{}
	done    chan struct{} // closed when the coordinator exits
	wg      sync.WaitGroup
	delay   time.Duration
	timeout time.Duration

	mu     sync.Mutex
	status map[string]TaskStatus

	running atomic.Int64
	max     atomic.Int64
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
		resize:  make(chan int, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		delay:   delay,
		timeout: timeout,
		status:  make(map[string]TaskStatus),
	}
	go p.coordinator(workers)
	p.max.Store(int64(workers))
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
	spawn(initial)

	for {
		select {
		case n := <-p.resize:
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
		case <-p.stop:
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

func (p *Pool) execute(id string) {
	p.setStatus(id, StatusRunning)
	p.running.Add(1)
	defer p.running.Add(-1)

	timer := time.NewTimer(p.delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		p.setStatus(id, StatusCompleted)
	case <-time.After(p.timeout):
		p.setStatus(id, StatusTimedOut)
	case <-p.stop:
		// Shutdown while a task is in-flight: do not overwrite its result.
		return
	}
}

// Submit enqueues a task. It returns an error if the pool is shutting down.
func (p *Pool) Submit(id string) error {
	if p.closed.Load() {
		return errors.New("pool closed")
	}
	p.setStatus(id, StatusQueued)
	select {
	case p.tasks <- id:
		return nil
	case <-p.stop:
		return errors.New("pool closed")
	}
}

// Resize changes the number of workers at runtime.
func (p *Pool) Resize(n int) {
	p.resize <- n
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

// Close shuts the pool down gracefully: in-flight tasks finish, no queued
// task is executed further.
func (p *Pool) Close() {
	if p.closed.Swap(true) {
		return
	}
	close(p.stop)
	<-p.done
	p.wg.Wait()
}

func (p *Pool) setStatus(id string, s TaskStatus) {
	p.mu.Lock()
	p.status[id] = s
	p.mu.Unlock()
}

// WaitAll blocks until all submitted tasks have reached a terminal status.
// Used for tests. ctx may be used to bound the wait.
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
