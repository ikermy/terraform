// Package grpc provides a gRPC mock server implementing the same domain
// operations as the HTTP mock (ClusterService and JobService).
package grpc

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"terraform-provider-ai/api/executor"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

type store struct {
	clusters map[string]*aiv1.Cluster
	jobs     map[string]*aiv1.Job
	mu       sync.Mutex
	seq      atomic.Uint64
}

func newStore() *store {
	return &store{
		clusters: make(map[string]*aiv1.Cluster),
		jobs:     make(map[string]*aiv1.Job),
	}
}

func (s *store) nextID(prefix, name string) string {
	return fmt.Sprintf("%s-%d-%s", prefix, s.seq.Add(1), name)
}

// Option configures the gRPC mock server's worker pool.
type Option func(*options)

type options struct {
	workers int
	delay   time.Duration
	timeout time.Duration
}

// WithWorkers sets the worker pool size used to execute jobs.
func WithWorkers(n int) Option { return func(o *options) { o.workers = n } }

// WithJobDelay sets the emulated execution time per job.
func WithJobDelay(d time.Duration) Option { return func(o *options) { o.delay = d } }

// WithJobTimeout sets the per-job deadline.
func WithJobTimeout(d time.Duration) Option { return func(o *options) { o.timeout = d } }

// Server implements aiv1.ClusterServiceServer and aiv1.JobServiceServer.
type Server struct {
	aiv1.UnimplementedClusterServiceServer
	aiv1.UnimplementedJobServiceServer
	store *store
	pool  *executor.Pool // executes submitted jobs
}

// NewServer returns a gRPC mock server. Jobs submitted via CreateJob are
// enqueued into an internal worker pool, and GetJob reflects the live pool
// status, matching the HTTP mock's behaviour.
func NewServer(opts ...Option) *Server {
	o := options{workers: 2, delay: 300 * time.Millisecond, timeout: 2 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}
	return &Server{
		store: newStore(),
		pool:  executor.NewPool(o.workers, o.delay, o.timeout),
	}
}

func (s *Server) CreateCluster(_ context.Context, req *aiv1.CreateClusterRequest) (*aiv1.Cluster, error) {
	c := req.GetCluster()
	if c == nil || c.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if c.Replicas < 0 {
		return nil, status.Error(codes.InvalidArgument, "replicas must be >= 0")
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	c.Id = s.store.nextID("cluster", c.Name)
	s.store.clusters[c.Id] = c
	return c, nil
}

func (s *Server) GetCluster(_ context.Context, req *aiv1.GetClusterRequest) (*aiv1.Cluster, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	c, ok := s.store.clusters[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "cluster not found")
	}
	return c, nil
}

func (s *Server) UpdateCluster(_ context.Context, req *aiv1.UpdateClusterRequest) (*aiv1.Cluster, error) {
	c := req.GetCluster()
	if c == nil || c.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if c.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if c.Replicas < 0 {
		return nil, status.Error(codes.InvalidArgument, "replicas must be >= 0")
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if _, ok := s.store.clusters[c.Id]; !ok {
		return nil, status.Error(codes.NotFound, "cluster not found")
	}
	s.store.clusters[c.Id] = c
	return c, nil
}

func (s *Server) DeleteCluster(_ context.Context, req *aiv1.DeleteClusterRequest) (*emptypb.Empty, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if _, ok := s.store.clusters[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, "cluster not found")
	}
	delete(s.store.clusters, req.GetId())
	return &emptypb.Empty{}, nil
}

func (s *Server) CreateJob(_ context.Context, req *aiv1.CreateJobRequest) (*aiv1.Job, error) {
	j := req.GetJob()
	if j == nil || j.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if j.Command == "" {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}
	if j.Priority < 0 {
		return nil, status.Error(codes.InvalidArgument, "priority must be >= 0")
	}

	s.store.mu.Lock()
	j.Id = s.store.nextID("job", j.Name)
	j.Status = "queued"
	s.store.jobs[j.Id] = j
	s.store.mu.Unlock()

	if err := s.pool.Submit(j.Id); err != nil {
		return nil, status.Error(codes.Internal, "failed to enqueue job: "+err.Error())
	}
	return j, nil
}

func (s *Server) GetJob(_ context.Context, req *aiv1.GetJobRequest) (*aiv1.Job, error) {
	s.store.mu.Lock()
	j, ok := s.store.jobs[req.GetId()]
	s.store.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.NotFound, "job not found")
	}
	if st := s.pool.Status(req.GetId()); st != "" {
		j.Status = string(st)
	}
	return j, nil
}

func (s *Server) UpdateJob(_ context.Context, req *aiv1.UpdateJobRequest) (*aiv1.Job, error) {
	j := req.GetJob()
	if j == nil || j.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if j.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if j.Command == "" {
		return nil, status.Error(codes.InvalidArgument, "command is required")
	}
	if j.Priority < 0 {
		return nil, status.Error(codes.InvalidArgument, "priority must be >= 0")
	}

	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if _, ok := s.store.jobs[j.Id]; !ok {
		return nil, status.Error(codes.NotFound, "job not found")
	}
	j.Status = "queued"
	s.store.jobs[j.Id] = j
	return j, nil
}

func (s *Server) DeleteJob(_ context.Context, req *aiv1.DeleteJobRequest) (*emptypb.Empty, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	if _, ok := s.store.jobs[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, "job not found")
	}
	delete(s.store.jobs, req.GetId())
	return &emptypb.Empty{}, nil
}
