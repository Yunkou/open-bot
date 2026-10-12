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
	got := s.purgeLangfuseUserTraces(context.Background(), "user-1", nil)
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
			w.Header().Set("Content-Type", "application/json")
			q := r.URL.Query()
			// Metadata fallback pass — primary already collected IDs; return empty.
			if q.Get("userId") == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
				return
			}
			mu.Lock()
			listCalls++
			page := listCalls
			sawUserID = q.Get("userId")
			mu.Unlock()
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
	got := s.purgeLangfuseUserTraces(context.Background(), "u1", nil)
	if !strings.HasPrefix(got, "deleted:3 traces") {
		t.Fatalf("got %q want deleted:3 traces…", got)
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
	got := s.purgeLangfuseUserTraces(context.Background(), "u1", nil)
	if !strings.HasPrefix(got, "list HTTP 502") {
		t.Fatalf("got %q", got)
	}
}

func TestPurgeLangfuseUserTracesUnreachable(t *testing.T) {
	withLangfuseEnv(t, "http://127.0.0.1:1", "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u1", nil)
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
	got := s.purgeLangfuseUserTraces(context.Background(), "u1", nil)
	if !strings.HasPrefix(got, "partial: deleted=0 delete_http=500") {
		t.Fatalf("got %q", got)
	}
}

func TestPurgeLangfuseUserTracesMetadataFallback(t *testing.T) {
	var mu sync.Mutex
	var sawUserIDFilter bool
	var sawMetaFilter bool
	var deleted []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v2/observations":
			w.Header().Set("Content-Type", "application/json")
			q := r.URL.Query()
			if q.Get("userId") == "u-meta" {
				mu.Lock()
				sawUserIDFilter = true
				mu.Unlock()
				// Primary userId filter finds nothing (trace only has metadata.user_id).
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
				return
			}
			if strings.Contains(q.Get("filter"), "user_id") && strings.Contains(q.Get("filter"), "u-meta") {
				mu.Lock()
				sawMetaFilter = true
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{
						{"id": "o1", "traceId": "tr-meta", "metadata": map[string]any{"user_id": "u-meta", "agent_id": "a1"}},
					},
					"meta": map[string]any{},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/traces":
			b, _ := io.ReadAll(r.Body)
			var body struct {
				TraceIDs []string `json:"traceIds"`
			}
			_ = json.Unmarshal(b, &body)
			mu.Lock()
			deleted = append(deleted, body.TraceIDs...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Traces deleted successfully"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u-meta", nil)
	if !strings.HasPrefix(got, "deleted:1 traces") {
		t.Fatalf("got %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawUserIDFilter || !sawMetaFilter {
		t.Fatalf("filters userId=%v meta=%v", sawUserIDFilter, sawMetaFilter)
	}
	if len(deleted) != 1 || deleted[0] != "tr-meta" {
		t.Fatalf("deleted=%v", deleted)
	}
}

func TestPurgeLangfuseUserTracesEmptyMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
	}))
	defer srv.Close()
	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "nobody", nil)
	if !strings.HasPrefix(got, "deleted:0 (no traces/scores") {
		t.Fatalf("got %q", got)
	}
}


func TestPurgeLangfuseUserTracesBySessionID(t *testing.T) {
	var mu sync.Mutex
	var sawSession string
	var deleted []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v2/observations":
			w.Header().Set("Content-Type", "application/json")
			q := r.URL.Query()
			if q.Get("userId") != "" || strings.Contains(q.Get("filter"), "user_id") {
				// Primary/metadata find nothing — only sessionId has the orphan span.
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
				return
			}
			if q.Get("sessionId") == "conv-1" {
				mu.Lock()
				sawSession = q.Get("sessionId")
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{
						{"id": "o1", "traceId": "tr-sess", "sessionId": "conv-1"}, // no userId
					},
					"meta": map[string]any{},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v3/scores":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/traces":
			b, _ := io.ReadAll(r.Body)
			var body struct {
				TraceIDs []string `json:"traceIds"`
			}
			_ = json.Unmarshal(b, &body)
			mu.Lock()
			deleted = append(deleted, body.TraceIDs...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Traces deleted successfully"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u-orphan", []string{"conv-1"})
	if !strings.HasPrefix(got, "deleted:1 traces") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "sessions:1") {
		t.Fatalf("expected sessions count in %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if sawSession != "conv-1" {
		t.Fatalf("sessionId=%q", sawSession)
	}
	if len(deleted) != 1 || deleted[0] != "tr-sess" {
		t.Fatalf("deleted=%v", deleted)
	}
}

func TestPurgeLangfuseSessionScoresDeleted(t *testing.T) {
	var mu sync.Mutex
	var deletedScores []string
	var deletedTraces []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v2/observations":
			q := r.URL.Query()
			if q.Get("userId") == "u-score" || q.Get("sessionId") == "sess-score" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{
						{"id": "o1", "traceId": "tr-1", "userId": "u-score", "sessionId": "sess-score"},
					},
					"meta": map[string]any{},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/v3/scores":
			if r.URL.Query().Get("sessionId") != "sess-score" {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "meta": map[string]any{}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "score-1", "name": "thumbs", "dataType": "BOOLEAN"},
					{"id": "score-2", "name": "rating", "dataType": "NUMERIC"},
				},
				"meta": map[string]any{},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/traces":
			b, _ := io.ReadAll(r.Body)
			var body struct {
				TraceIDs []string `json:"traceIds"`
			}
			_ = json.Unmarshal(b, &body)
			mu.Lock()
			deletedTraces = append(deletedTraces, body.TraceIDs...)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/public/scores/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/public/scores/")
			mu.Lock()
			deletedScores = append(deletedScores, id)
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Score deletion queued successfully"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withLangfuseEnv(t, srv.URL, "pk", "sk")
	s := &Server{}
	got := s.purgeLangfuseUserTraces(context.Background(), "u-score", []string{"sess-score"})
	if !strings.Contains(got, "deleted:1 traces") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "scores:2") {
		t.Fatalf("expected scores:2 in %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deletedTraces) != 1 || deletedTraces[0] != "tr-1" {
		t.Fatalf("traces=%v", deletedTraces)
	}
	want := map[string]bool{"score-1": true, "score-2": true}
	for _, id := range deletedScores {
		if !want[id] {
			t.Fatalf("unexpected score %s", id)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing score deletes: %v", want)
	}
}
