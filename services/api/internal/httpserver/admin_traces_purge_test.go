package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func withLangfuseEnv(t *testing.T, base, pk, sk string) {
	t.Helper()
	keys := []string{
		"LANGFUSE_BASE_URL",
		"LANGFUSE_PUBLIC_KEY",
		"LANGFUSE_SECRET_KEY",
		"LANGFUSE_ENABLED",
		"LANGFUSE_PROJECT_ID",
		"LANGFUSE_PUBLIC_UI_URL",
	}
	prev := map[string]string{}
	for _, k := range keys {
		prev[k], _ = os.LookupEnv(k)
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			if v, ok := prev[k]; ok && v != "" {
				_ = os.Setenv(k, v)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	})
	if base != "" {
		t.Setenv("LANGFUSE_BASE_URL", base)
	}
	if pk != "" {
		t.Setenv("LANGFUSE_PUBLIC_KEY", pk)
	}
	if sk != "" {
		t.Setenv("LANGFUSE_SECRET_KEY", sk)
	}
}

func TestPurgeLangfuseUserTracesSkippedWhenUnconfigured(t *testing.T) {
	withLangfuseEnv(t, "", "", "")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "user-1")
	if !strings.HasPrefix(got, "skipped:") {
		t.Fatalf("expected skipped, got %q", got)
	}
}

func TestPurgeLangfuseUserTracesListsAndBatchDeletes(t *testing.T) {
	var mu sync.Mutex
	var listCalls int
	var deleted []string
	var sawUserID string
	var sawBasicAuth bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if ok && user == "pk-test" && pass == "sk-test" {
			mu.Lock()
			sawBasicAuth = true
			mu.Unlock()
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v2/observations":
			mu.Lock()
			listCalls++
			page := listCalls
			sawUserID = r.URL.Query().Get("userId")
			mu.Unlock()
			if r.URL.Query().Get("isRootObservation") != "true" {
				t.Errorf("expected isRootObservation=true")
			}
			w.Header().Set("Content-Type", "application/json")
			if page == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{
						{"id": "obs-1", "traceId": "tr-a", "userId": "u1"},
						{"id": "obs-2", "traceId": "tr-b", "userId": "u1"},
						{"id": "obs-3", "traceId": "tr-a", "userId": "u1"}, // dup
					},
					"meta": map[string]any{"cursor": "page2"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "obs-4", "traceId": "tr-c", "userId": "u1"},
				},
				"meta": map[string]any{},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/traces":
			b, _ := io.ReadAll(r.Body)
			var body struct {
				TraceIDs []string `json:"traceIds"`
			}
			if err := json.Unmarshal(b, &body); err != nil {
				t.Errorf("delete body: %v", err)
				http.Error(w, "bad json", 400)
				return
			}
			mu.Lock()
			deleted = append(deleted, body.TraceIDs...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Traces deleted successfully"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withLangfuseEnv(t, srv.URL, "pk-test", "sk-test")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u1")
	if got != "purged:3" {
		t.Fatalf("got %q want purged:3", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawBasicAuth {
		t.Fatal("expected basic auth")
	}
	if sawUserID != "u1" {
		t.Fatalf("userId query=%q", sawUserID)
	}
	if listCalls != 2 {
		t.Fatalf("listCalls=%d", listCalls)
	}
	if len(deleted) != 3 {
		t.Fatalf("deleted=%v", deleted)
	}
	want := map[string]bool{"tr-a": true, "tr-b": true, "tr-c": true}
	for _, id := range deleted {
		if !want[id] {
			t.Fatalf("unexpected id %s in %v", id, deleted)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing deletes: %v", want)
	}
}

func TestPurgeLangfuseUserTracesBestEffortOnListDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u1")
	if !strings.HasPrefix(got, "list HTTP 502") {
		t.Fatalf("got %q", got)
	}
}

func TestPurgeLangfuseUserTracesUnreachable(t *testing.T) {
	withLangfuseEnv(t, "http://127.0.0.1:1", "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u1")
	if !strings.HasPrefix(got, "unreachable:") {
		t.Fatalf("got %q", got)
	}
}

func TestPurgeLangfuseUserTracesPartialOnDeleteFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "o1", "traceId": "tr-1"},
					{"id": "o2", "traceId": "tr-2"},
				},
				"meta": map[string]any{},
			})
			return
		}
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u1")
	if !strings.HasPrefix(got, "partial: deleted=0 delete_http=500") {
		t.Fatalf("got %q", got)
	}
}
