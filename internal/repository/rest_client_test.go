package repository

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"terraform-provider-ai/api"
	"terraform-provider-ai/internal/entity"
)

type atomicCounter struct{ atomic.Int64 }

func (c *atomicCounter) Add(delta int64) int64 { return c.Int64.Add(delta) }
func (c *atomicCounter) Load() int64           { return c.Int64.Load() }

func newTestClient(t *testing.T) *RestClient {
	t.Helper()
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)
	return NewRestClient(srv.URL, "test-token")
}

func TestRestClient_CRUD(t *testing.T) {
	client := newTestClient(t)

	created, err := client.Create(context.Background(), &entity.Cluster{Name: "demo", Replicas: 2, Model: "gpt-mini"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a non-empty id")
	}

	got, err := client.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got == nil || got.Name != "demo" {
		t.Fatalf("unexpected get result: %+v", got)
	}

	updated, err := client.Update(context.Background(), &entity.Cluster{ID: created.ID, Name: "demo", Replicas: 5, Model: "gpt-large"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Replicas != 5 {
		t.Fatalf("expected replicas 5, got %d", updated.Replicas)
	}

	if err := client.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = client.Get(context.Background(), created.ID)
	if !errors.Is(err, entity.ErrClusterNotFound) {
		t.Fatalf("expected ErrClusterNotFound after delete, got %v", err)
	}
}

func TestRestClient_CreateIdempotent(t *testing.T) {
	client := newTestClient(t)

	first, err := client.Create(context.Background(), &entity.Cluster{Name: "idem", Replicas: 1, Model: "gpt-mini"})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	second, err := client.Create(context.Background(), &entity.Cluster{Name: "idem", Replicas: 1, Model: "gpt-mini"})
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same id for idempotent create, got %q and %q", first.ID, second.ID)
	}
}

func TestRestClient_GetNotFound(t *testing.T) {
	client := newTestClient(t)

	_, err := client.Get(context.Background(), "cluster-missing")
	if !errors.Is(err, entity.ErrClusterNotFound) {
		t.Fatalf("expected ErrClusterNotFound, got %v", err)
	}
}

func TestRestClient_RetriesOn5xx(t *testing.T) {
	var calls atomicCounter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cluster-1-demo","name":"demo","replicas":0}`))
	}))
	defer srv.Close()

	client := NewRestClient(srv.URL, "test-token")
	_, err := client.Create(context.Background(), &entity.Cluster{Name: "demo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestRestClient_UnauthorizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewRestClient(srv.URL, "test-token")
	_, err := client.Create(context.Background(), &entity.Cluster{Name: "demo"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}
