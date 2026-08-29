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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"terraform-provider-ai/api/executor"
	"terraform-provider-ai/config"
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
	timeout time.Duration
}

// WithWorkers sets the worker pool size used to execute jobs.
func WithWorkers(n int) Option { return func(o *options) { o.workers = n } }

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
	o := options{workers: config.DefaultWorkers, timeout: config.DefaultJobTimeout}
	for _, opt := range opts {
		opt(&o)
	}
	return &Server{
		store: newStore(),
		pool:  executor.NewPool(o.workers, executor.ExecWork, o.timeout),
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

	c = proto.Clone(c).(*aiv1.Cluster)
	c.Id = s.store.nextID("cluster", c.Name)
	s.store.clusters[c.Id] = c
	return proto.Clone(c).(*aiv1.Cluster), nil
}

func (s *Server) GetCluster(_ context.Context, req *aiv1.GetClusterRequest) (*aiv1.Cluster, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	c, ok := s.store.clusters[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "cluster not found")
	}
	// Return a copy so callers cannot mutate the stored object.
	return proto.Clone(c).(*aiv1.Cluster), nil
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
	// Store a copy so a mutating caller cannot corrupt the store.
	s.store.clusters[c.Id] = proto.Clone(c).(*aiv1.Cluster)
	return proto.Clone(c).(*aiv1.Cluster), nil
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
	j = proto.Clone(j).(*aiv1.Job)
	j.Id = s.store.nextID("job", j.Name)
	j.Status = "queued"
	s.store.jobs[j.Id] = proto.Clone(j).(*aiv1.Job)
	s.store.mu.Unlock()

	if err := s.pool.Submit(j.Id, j.Command); err != nil {
		return nil, status.Error(codes.Internal, "failed to enqueue job: "+err.Error())
	}
	return proto.Clone(j).(*aiv1.Job), nil
}

func (s *Server) GetJob(_ context.Context, req *aiv1.GetJobRequest) (*aiv1.Job, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	j, ok := s.store.jobs[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "job not found")
	}
	// Clone before mutating status so parallel GetJob calls never race on the
	// stored object.
	cloned := proto.Clone(j).(*aiv1.Job)
	if st := s.pool.Status(req.GetId()); st != "" {
		cloned.Status = string(st)
	}
	return cloned, nil
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
	// Store a copy so a mutating caller cannot corrupt the store.
	j = proto.Clone(j).(*aiv1.Job)
	j.Status = "queued"
	s.store.jobs[j.Id] = proto.Clone(j).(*aiv1.Job)
	return proto.Clone(j).(*aiv1.Job), nil
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
