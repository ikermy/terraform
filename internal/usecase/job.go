package usecase

import (
	"context"

	"terraform-provider-ai/internal/entity"
	"terraform-provider-ai/internal/repository"
)

type JobInteractor struct {
	repo repository.JobRepository
}

func NewJobInteractor(repo repository.JobRepository) *JobInteractor {
	return &JobInteractor{repo: repo}
}

func (ji *JobInteractor) CreateJob(ctx context.Context, j entity.Job) (*entity.Job, error) {
	if err := validateJob(j); err != nil {
		return nil, err
	}
	return ji.repo.CreateJob(ctx, &j)
}

func (ji *JobInteractor) GetJob(ctx context.Context, id string) (*entity.Job, error) {
	return ji.repo.GetJob(ctx, id)
}

func (ji *JobInteractor) UpdateJob(ctx context.Context, j entity.Job) (*entity.Job, error) {
	if j.ID == "" {
		return nil, entity.ErrJobIDRequired
	}
	if err := validateJob(j); err != nil {
		return nil, err
	}
	return ji.repo.UpdateJob(ctx, &j)
}

func (ji *JobInteractor) DeleteJob(ctx context.Context, id string) error {
	return ji.repo.DeleteJob(ctx, id)
}

func validateJob(j entity.Job) error {
	if j.Name == "" {
		return entity.ErrJobNameRequired
	}
	if j.Priority < 0 {
		return entity.ErrJobNegativePri
	}
	return nil
}
