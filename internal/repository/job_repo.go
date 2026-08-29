package repository

import (
	"context"

	"terraform-provider-ai/internal/entity"
)

type JobRepository interface {
	CreateJob(ctx context.Context, job *entity.Job) (*entity.Job, error)
	GetJob(ctx context.Context, id string) (*entity.Job, error)
	UpdateJob(ctx context.Context, job *entity.Job) (*entity.Job, error)
	DeleteJob(ctx context.Context, id string) error
}
