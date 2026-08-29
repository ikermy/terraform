package repository

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	mockgrpc "terraform-provider-ai/api/grpc"
	"terraform-provider-ai/internal/entity"
	aiv1 "terraform-provider-ai/proto/ai/v1"
)

const bufSize = 1024 * 1024

func newBufconnClient(t *testing.T) *GrpcClient {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	s := grpc.NewServer()
	mock := mockgrpc.NewServer()
	aiv1.RegisterClusterServiceServer(s, mock)
	aiv1.RegisterJobServiceServer(s, mock)
	go func() {
		if err := s.Serve(lis); err != nil {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	return newGrpcClient(conn)
}

func TestGrpcClient_ClusterCRUD(t *testing.T) {
	client := newBufconnClient(t)
	ctx := context.Background()

	created, err := client.Create(ctx, &entity.Cluster{Name: "demo", Replicas: 2, Model: "gpt-mini"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.Name != "demo" {
		t.Fatalf("unexpected created: %+v", created)
	}

	got, err := client.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Replicas != 2 {
		t.Fatalf("expected replicas 2, got %d", got.Replicas)
	}

	updated, err := client.Update(ctx, &entity.Cluster{ID: created.ID, Name: "demo", Replicas: 5, Model: "gpt-large"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Replicas != 5 {
		t.Fatalf("expected replicas 5, got %d", updated.Replicas)
	}

	if err := client.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := client.Get(ctx, created.ID); !errors.Is(err, entity.ErrClusterNotFound) {
		t.Fatalf("expected ErrClusterNotFound, got %v", err)
	}
}

func TestGrpcClient_JobCRUD(t *testing.T) {
	client := newBufconnClient(t)
	ctx := context.Background()

	created, err := client.CreateJob(ctx, &entity.Job{Name: "demo", Command: "echo hi", Priority: 3})
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	if created.Status == "" {
		t.Fatal("expected status to be set")
	}

	got, err := client.GetJob(ctx, created.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Priority != 3 {
		t.Fatalf("expected priority 3, got %d", got.Priority)
	}

	if err := client.DeleteJob(ctx, created.ID); err != nil {
		t.Fatalf("delete job: %v", err)
	}
	if _, err := client.GetJob(ctx, created.ID); !errors.Is(err, entity.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
}

func TestGrpcClient_ValidationError(t *testing.T) {
	client := newBufconnClient(t)

	_, err := client.Create(context.Background(), &entity.Cluster{Name: ""})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
