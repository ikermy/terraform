package usecase

import (
	"context"
	"errors"
	"testing"

	"terraform-provider-ai/internal/entity"
)

type mockRepo struct {
	createFn func(context.Context, *entity.Cluster) (*entity.Cluster, error)
	getFn    func(context.Context, string) (*entity.Cluster, error)
	updateFn func(context.Context, *entity.Cluster) (*entity.Cluster, error)
	deleteFn func(context.Context, string) error
}

func (m *mockRepo) Create(ctx context.Context, c *entity.Cluster) (*entity.Cluster, error) {
	return m.createFn(ctx, c)
}
func (m *mockRepo) Get(ctx context.Context, id string) (*entity.Cluster, error) {
	return m.getFn(ctx, id)
}
func (m *mockRepo) Update(ctx context.Context, c *entity.Cluster) (*entity.Cluster, error) {
	return m.updateFn(ctx, c)
}
func (m *mockRepo) Delete(ctx context.Context, id string) error { return m.deleteFn(ctx, id) }

func TestCreateCluster_Valid(t *testing.T) {
	ctx := context.Background()
	repo := &mockRepo{
		createFn: func(_ context.Context, c *entity.Cluster) (*entity.Cluster, error) {
			c.ID = "cluster-" + c.Name
			return c, nil
		},
	}
	interactor := NewClusterInteractor(repo)

	got, err := interactor.CreateCluster(ctx, entity.Cluster{Name: "demo", Replicas: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "cluster-demo" {
		t.Fatalf("expected id cluster-demo, got %s", got.ID)
	}
}

func TestCreateCluster_EmptyName(t *testing.T) {
	interactor := NewClusterInteractor(&mockRepo{})

	_, err := interactor.CreateCluster(context.Background(), entity.Cluster{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateCluster_RepoError(t *testing.T) {
	repo := &mockRepo{
		createFn: func(_ context.Context, c *entity.Cluster) (*entity.Cluster, error) {
			return nil, errors.New("boom")
		},
	}
	interactor := NewClusterInteractor(repo)

	_, err := interactor.CreateCluster(context.Background(), entity.Cluster{Name: "demo"})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestCreateCluster_NormalizesModel(t *testing.T) {
	var got *entity.Cluster
	repo := &mockRepo{
		createFn: func(_ context.Context, c *entity.Cluster) (*entity.Cluster, error) {
			got = c
			c.ID = "cluster-demo"
			return c, nil
		},
	}
	interactor := NewClusterInteractor(repo)

	_, err := interactor.CreateCluster(context.Background(), entity.Cluster{Name: "demo", Model: "GPT-MINI"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Model != "gpt-mini" {
		t.Fatalf("expected model normalized to gpt-mini, got %q", got.Model)
	}
}

func TestCreateCluster_NegativeReplicas(t *testing.T) {
	interactor := NewClusterInteractor(&mockRepo{})

	_, err := interactor.CreateCluster(context.Background(), entity.Cluster{Name: "demo", Replicas: -1})
	if !errors.Is(err, entity.ErrClusterNegativeReps) {
		t.Fatalf("expected ErrClusterNegativeReps, got %v", err)
	}
}
