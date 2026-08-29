package usecase

import (
	"context"
	"strings"

	"golang.org/x/sync/errgroup"

	"terraform-provider-ai/config"
	"terraform-provider-ai/internal/entity"
	"terraform-provider-ai/internal/repository"
)

type ClusterInteractor struct {
	repo repository.ClusterRepository
}

func NewClusterInteractor(repo repository.ClusterRepository) *ClusterInteractor {
	return &ClusterInteractor{repo: repo}
}

func (ci *ClusterInteractor) CreateCluster(ctx context.Context, c entity.Cluster) (*entity.Cluster, error) {
	if err := validateCluster(c); err != nil {
		return nil, err
	}
	normalize(&c)
	return ci.repo.Create(ctx, &c)
}

func (ci *ClusterInteractor) GetCluster(ctx context.Context, id string) (*entity.Cluster, error) {
	return ci.repo.Get(ctx, id)
}

func (ci *ClusterInteractor) UpdateCluster(ctx context.Context, c entity.Cluster) (*entity.Cluster, error) {
	if c.ID == "" {
		return nil, entity.ErrClusterIDRequired
	}
	if err := validateCluster(c); err != nil {
		return nil, err
	}
	normalize(&c)
	return ci.repo.Update(ctx, &c)
}

func (ci *ClusterInteractor) DeleteCluster(ctx context.Context, id string) error {
	return ci.repo.Delete(ctx, id)
}

// BatchCreateClusters creates multiple clusters concurrently and returns
// the created clusters in the same order as the input. It stops at the first
// error, cancelling the remaining in-flight creates.
//
// Concurrency is bounded with g.SetLimit to avoid overwhelming the upstream
// API (socket exhaustion / rate limiting) on large batches.
func (ci *ClusterInteractor) BatchCreateClusters(ctx context.Context, clusters []entity.Cluster) ([]*entity.Cluster, error) {
	created := make([]*entity.Cluster, len(clusters))

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(config.BatchConcurrencyLimit) // bounded concurrent API requests
	for i := range clusters {
		i := i
		c := clusters[i]
		g.Go(func() error {
			out, err := ci.CreateCluster(gctx, c)
			if err != nil {
				return err
			}
			created[i] = out
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return created, nil
}

// validateCluster applies business rules shared by Create and Update.
func validateCluster(c entity.Cluster) error {
	if strings.TrimSpace(c.Name) == "" {
		return entity.ErrClusterNameRequired
	}
	if c.Replicas < 0 {
		return entity.ErrClusterNegativeReps
	}
	return nil
}

// normalize brings fields to a canonical form (model is case-insensitive).
func normalize(c *entity.Cluster) {
	c.Model = strings.ToLower(c.Model)
}
