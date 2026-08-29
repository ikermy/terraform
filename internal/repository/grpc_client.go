package repository

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"terraform-provider-ai/internal/entity"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

// GrpcClient implements ClusterRepository and JobRepository over gRPC.
// It maps the domain entities to/from the generated proto types, so the
// domain layer never depends on the gRPC/protobuf types.
type GrpcClient struct {
	conn     grpc.ClientConnInterface
	clusters aiv1.ClusterServiceClient
	jobs     aiv1.JobServiceClient
}

func NewGrpcClient(target string) (*GrpcClient, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	return newGrpcClient(conn), nil
}

// newGrpcClient builds a client from an existing connection (used in tests
// with in-process bufconn servers).
func newGrpcClient(conn grpc.ClientConnInterface) *GrpcClient {
	return &GrpcClient{
		conn:     conn,
		clusters: aiv1.NewClusterServiceClient(conn),
		jobs:     aiv1.NewJobServiceClient(conn),
	}
}

// Close releases the underlying connection.
func (g *GrpcClient) Close() error {
	if c, ok := g.conn.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

func (g *GrpcClient) Create(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error) {
	out, err := g.clusters.CreateCluster(ctx, &aiv1.CreateClusterRequest{
		Cluster: clusterToPB(cluster),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return clusterFromPB(out), nil
}

func (g *GrpcClient) Get(ctx context.Context, id string) (*entity.Cluster, error) {
	out, err := g.clusters.GetCluster(ctx, &aiv1.GetClusterRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, entity.ErrClusterNotFound
		}
		return nil, grpcError(err)
	}
	return clusterFromPB(out), nil
}

func (g *GrpcClient) Update(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error) {
	out, err := g.clusters.UpdateCluster(ctx, &aiv1.UpdateClusterRequest{
		Cluster: clusterToPB(cluster),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return clusterFromPB(out), nil
}

func (g *GrpcClient) Delete(ctx context.Context, id string) error {
	_, err := g.clusters.DeleteCluster(ctx, &aiv1.DeleteClusterRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return entity.ErrClusterNotFound
		}
		return grpcError(err)
	}
	return nil
}

func (g *GrpcClient) CreateJob(ctx context.Context, job *entity.Job) (*entity.Job, error) {
	out, err := g.jobs.CreateJob(ctx, &aiv1.CreateJobRequest{Job: jobToPB(job)})
	if err != nil {
		return nil, grpcError(err)
	}
	return jobFromPB(out), nil
}

func (g *GrpcClient) GetJob(ctx context.Context, id string) (*entity.Job, error) {
	out, err := g.jobs.GetJob(ctx, &aiv1.GetJobRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, entity.ErrJobNotFound
		}
		return nil, grpcError(err)
	}
	return jobFromPB(out), nil
}

func (g *GrpcClient) UpdateJob(ctx context.Context, job *entity.Job) (*entity.Job, error) {
	out, err := g.jobs.UpdateJob(ctx, &aiv1.UpdateJobRequest{Job: jobToPB(job)})
	if err != nil {
		return nil, grpcError(err)
	}
	return jobFromPB(out), nil
}

func (g *GrpcClient) DeleteJob(ctx context.Context, id string) error {
	_, err := g.jobs.DeleteJob(ctx, &aiv1.DeleteJobRequest{Id: id})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return entity.ErrJobNotFound
		}
		return grpcError(err)
	}
	return nil
}

// grpcError normalizes unexpected gRPC errors, preserving sentinel identity
// via errors.Is where applicable.
func grpcError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, entity.ErrClusterNotFound) || errors.Is(err, entity.ErrJobNotFound) {
		return err
	}
	return fmt.Errorf("grpc: %w", err)
}

func clusterToPB(c *entity.Cluster) *aiv1.Cluster {
	return &aiv1.Cluster{
		Id:       c.ID,
		Name:     c.Name,
		Replicas: int32(c.Replicas),
		Model:    c.Model,
	}
}

func clusterFromPB(c *aiv1.Cluster) *entity.Cluster {
	return &entity.Cluster{
		ID:       c.GetId(),
		Name:     c.GetName(),
		Replicas: int(c.GetReplicas()),
		Model:    c.GetModel(),
	}
}

func jobToPB(j *entity.Job) *aiv1.Job {
	return &aiv1.Job{
		Id:       j.ID,
		Name:     j.Name,
		Command:  j.Command,
		Priority: int32(j.Priority),
		Status:   j.Status,
	}
}

func jobFromPB(j *aiv1.Job) *entity.Job {
	return &entity.Job{
		ID:       j.GetId(),
		Name:     j.GetName(),
		Command:  j.GetCommand(),
		Priority: int(j.GetPriority()),
		Status:   j.GetStatus(),
	}
}
