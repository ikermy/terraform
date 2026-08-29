package repository

import (
	"context"

	"terraform-provider-ai/internal/entity"
)

type ClusterRepository interface {
	Create(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error)
	Get(ctx context.Context, id string) (*entity.Cluster, error)
	Update(ctx context.Context, cluster *entity.Cluster) (*entity.Cluster, error)
	Delete(ctx context.Context, id string) error
}
