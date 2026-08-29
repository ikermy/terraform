package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewServer())
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url string, body []byte) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	var out map[string]any
	if resp.StatusCode < 300 && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp, out
}

func TestServer_CRUD(t *testing.T) {
	srv := newTestServer(t)

	resp, created := do(t, http.MethodPost, srv.URL+"/clusters", []byte(`{"name":"demo","replicas":3,"model":"gpt-mini"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create status: %d", resp.StatusCode)
	}
	if created["id"] == nil || created["name"] != "demo" {
		t.Fatalf("unexpected created: %+v", created)
	}
	id := created["id"].(string)

	resp, got := do(t, http.MethodGet, srv.URL+"/clusters/"+id, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status: %d", resp.StatusCode)
	}
	if got["name"] != "demo" {
		t.Fatalf("unexpected get: %+v", got)
	}

	resp, updated := do(t, http.MethodPut, srv.URL+"/clusters/"+id, []byte(`{"name":"demo","replicas":5,"model":"gpt-large"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status: %d", resp.StatusCode)
	}
	if updated["replicas"] != float64(5) {
		t.Fatalf("unexpected update: %+v", updated)
	}

	resp, _ = do(t, http.MethodDelete, srv.URL+"/clusters/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status: %d", resp.StatusCode)
	}

	resp, _ = do(t, http.MethodGet, srv.URL+"/clusters/"+id, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", resp.StatusCode)
	}
}

func TestServer_Validation(t *testing.T) {
	srv := newTestServer(t)

	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{"empty name", `{"replicas":1}`, http.StatusBadRequest},
		{"negative replicas", `{"name":"x","replicas":-1}`, http.StatusBadRequest},
		{"invalid json", `{`, http.StatusBadRequest},
	} {
		resp, _ := do(t, http.MethodPost, srv.URL+"/clusters", []byte(tc.body))
		if resp.StatusCode != tc.code {
			t.Fatalf("%s: expected %d, got %d", tc.name, resp.StatusCode, tc.code)
		}
	}
}

func TestServer_Idempotency(t *testing.T) {
	srv := newTestServer(t)

	create := func() string {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/clusters", bytes.NewReader([]byte(`{"name":"idem","replicas":1}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "k1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return out["id"].(string)
	}

	first := create()
	second := create()
	if first != second {
		t.Fatalf("expected same id for idempotent create, got %q and %q", first, second)
	}
}

func TestServer_JobRunsThroughPool(t *testing.T) {
	srv := newTestServer(t)

	resp, created := do(t, http.MethodPost, srv.URL+"/jobs", []byte(`{"name":"job1","command":"echo hi","priority":1}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create job status: %d", resp.StatusCode)
	}
	if created["status"] != "queued" {
		t.Fatalf("expected initial status queued, got %q", created["status"])
	}
	id := created["id"].(string)

	// The pool should transition the job to a terminal status shortly.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, got := do(t, http.MethodGet, srv.URL+"/jobs/"+id, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get job status: %d", resp.StatusCode)
		}
		status := got["status"].(string)
		if status == "completed" || status == "timed_out" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not reach a terminal status in time")
}

func TestServer_List(t *testing.T) {
	srv := newTestServer(t)

	do(t, http.MethodPost, srv.URL+"/clusters", []byte(`{"name":"a","replicas":1}`))
	do(t, http.MethodPost, srv.URL+"/clusters", []byte(`{"name":"b","replicas":2}`))

	resp, err := http.Get(srv.URL + "/clusters")
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status: %d", resp.StatusCode)
	}

	var list []Cluster
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(list))
	}
}
