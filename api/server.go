package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"terraform-provider-ai/api/executor"
)

type Cluster struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
	Model    string `json:"model"`
}

type Job struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Priority int    `json:"priority"`
	Status   string `json:"status"`
}

// ServerOption configures the mock API server.
type ServerOption func(*serverOptions)

type serverOptions struct {
	workers int
	timeout time.Duration
}

// WithWorkers sets the worker pool size used to execute jobs.
func WithWorkers(n int) ServerOption {
	return func(o *serverOptions) { o.workers = n }
}

// WithJobTimeout sets the per-job deadline.
func WithJobTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.timeout = d }
}

type store struct {
	clusters map[string]Cluster
	keys     map[string]string // Idempotency-Key -> cluster ID
	jobs     map[string]Job
	jobKeys  map[string]string // Idempotency-Key -> job ID
	pool     *executor.Pool    // executes submitted jobs
	mu       sync.Mutex
	seq      atomic.Uint64
}

func newStore(pool *executor.Pool) *store {
	return &store{
		clusters: make(map[string]Cluster),
		keys:     make(map[string]string),
		jobs:     make(map[string]Job),
		jobKeys:  make(map[string]string),
		pool:     pool,
	}
}

// NewServer returns an http.Handler. Jobs submitted via POST /jobs are
// enqueued into an internal worker pool, and GET /jobs/<id> reflects the live
// pool status (queued -> running -> completed|timed_out).
func NewServer(opts ...ServerOption) http.Handler {
	o := serverOptions{workers: 2, timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}
	pool := executor.NewPool(o.workers, executor.ExecWork, o.timeout)
	s := newStore(pool)
	mux := http.NewServeMux()
	mux.HandleFunc("/clusters", s.handleClusters)
	mux.HandleFunc("/clusters/", s.handleClusterByID)
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/jobs/", s.handleJobByID)
	return mux
}

func (s *store) nextID(prefix, name string) string {
	return fmt.Sprintf("%s-%d-%s", prefix, s.seq.Add(1), name)
}

func (s *store) handleClusters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var c Cluster
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if c.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if c.Replicas < 0 {
			http.Error(w, "replicas must be >= 0", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		// Idempotency: return the already-created cluster for the same key.
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			if existingID, ok := s.keys[key]; ok {
				existing := s.clusters[existingID]
				s.mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(existing)
				return
			}
			c.ID = s.nextID("cluster", c.Name)
			s.clusters[c.ID] = c
			s.keys[key] = c.ID
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(c)
			return
		}

		c.ID = s.nextID("cluster", c.Name)
		s.clusters[c.ID] = c
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(c)
	case http.MethodGet:
		s.mu.Lock()
		defer s.mu.Unlock()
		var list []Cluster
		for _, c := range s.clusters {
			list = append(list, c)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *store) handleClusterByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/clusters/"):]
	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		c, ok := s.clusters[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	case http.MethodPut:
		var c Cluster
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if c.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if c.Replicas < 0 {
			http.Error(w, "replicas must be >= 0", http.StatusBadRequest)
			return
		}
		if _, exists := s.clusters[id]; !exists {
			http.NotFound(w, r)
			return
		}
		c.ID = id
		s.clusters[id] = c
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(c)
	case http.MethodDelete:
		if _, exists := s.clusters[id]; !exists {
			http.NotFound(w, r)
			return
		}
		delete(s.clusters, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *store) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var j Job
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if j.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if j.Command == "" {
			http.Error(w, "command is required", http.StatusBadRequest)
			return
		}
		if j.Priority < 0 {
			http.Error(w, "priority must be >= 0", http.StatusBadRequest)
			return
		}

		s.mu.Lock()
		existingID := ""
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			existingID = s.jobKeys[key]
		}
		if existingID != "" {
			existing := s.jobs[existingID]
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(existing)
			return
		}

		j.ID = s.nextID("job", j.Name)
		j.Status = "queued"
		s.jobs[j.ID] = j
		if key := r.Header.Get("Idempotency-Key"); key != "" {
			s.jobKeys[key] = j.ID
		}
		s.mu.Unlock()

		// Enqueue the job into the worker pool; GET reflects its live status.
		if err := s.pool.Submit(j.ID, j.Command); err != nil {
			http.Error(w, "failed to enqueue job: "+err.Error(), http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(j)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *store) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/jobs/"):]
	s.mu.Lock()
	defer s.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		j, ok := s.jobs[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if st := s.pool.Status(id); st != "" {
			j.Status = string(st)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(j)
	case http.MethodPut:
		var j Job
		if err := json.NewDecoder(r.Body).Decode(&j); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if j.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		if j.Command == "" {
			http.Error(w, "command is required", http.StatusBadRequest)
			return
		}
		if j.Priority < 0 {
			http.Error(w, "priority must be >= 0", http.StatusBadRequest)
			return
		}
		if _, exists := s.jobs[id]; !exists {
			http.NotFound(w, r)
			return
		}
		j.ID = id
		j.Status = "queued"
		s.jobs[id] = j
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(j)
	case http.MethodDelete:
		if _, exists := s.jobs[id]; !exists {
			http.NotFound(w, r)
			return
		}
		delete(s.jobs, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
