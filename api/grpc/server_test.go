package grpc

import (
	"context"
	"testing"
	"time"

	aiv1 "terraform-provider-ai/proto/ai/v1"
)

func TestGRPCServer_ClusterCRUD(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	created, err := s.CreateCluster(ctx, &aiv1.CreateClusterRequest{
		Cluster: &aiv1.Cluster{Name: "demo", Replicas: 2, Model: "gpt-mini"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetId() == "" {
		t.Fatal("expected non-empty id")
	}

	got, err := s.GetCluster(ctx, &aiv1.GetClusterRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetName() != "demo" {
		t.Fatalf("unexpected name %q", got.GetName())
	}

	_, err = s.UpdateCluster(ctx, &aiv1.UpdateClusterRequest{
		Cluster: &aiv1.Cluster{Id: created.GetId(), Name: "demo", Replicas: 5, Model: "gpt-large"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if _, err := s.DeleteCluster(ctx, &aiv1.DeleteClusterRequest{Id: created.GetId()}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetCluster(ctx, &aiv1.GetClusterRequest{Id: created.GetId()}); err == nil {
		t.Fatal("expected NotFound after delete")
	}
}

func TestGRPCServer_ClusterValidation(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	for name, req := range map[string]*aiv1.CreateClusterRequest{
		"empty name":    {Cluster: &aiv1.Cluster{Name: "", Replicas: 1}},
		"negative reps": {Cluster: &aiv1.Cluster{Name: "x", Replicas: -1}},
	} {
		if _, err := s.CreateCluster(ctx, req); err == nil {
			t.Fatalf("expected error for %s", name)
		}
	}
}

func TestGRPCServer_JobRunsThroughPool(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	created, err := s.CreateJob(ctx, &aiv1.CreateJobRequest{
		Job: &aiv1.Job{Name: "job1", Command: "echo ok", Priority: 1},
	})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if created.GetStatus() != "queued" {
		t.Fatalf("expected initial status queued, got %q", created.GetStatus())
	}

	// The worker pool should drive the job to a terminal status.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := s.GetJob(ctx, &aiv1.GetJobRequest{Id: created.GetId()})
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		switch got.GetStatus() {
		case "completed", "failed", "timed_out":
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not reach a terminal status in time")
}

func TestGRPCServer_JobValidation(t *testing.T) {
	s := NewServer()
	ctx := context.Background()

	for name, req := range map[string]*aiv1.CreateJobRequest{
		"empty name":    {Job: &aiv1.Job{Name: "", Command: "echo", Priority: 1}},
		"empty command": {Job: &aiv1.Job{Name: "x", Command: "", Priority: 1}},
		"neg priority":  {Job: &aiv1.Job{Name: "x", Command: "echo", Priority: -1}},
	} {
		if _, err := s.CreateJob(ctx, req); err == nil {
			t.Fatalf("expected error for %s", name)
		}
	}
}
