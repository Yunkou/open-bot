package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolvePromptCacheHeuristic(t *testing.T) {
	if got := resolvePromptCacheHeuristic("claude-3-5", "https://api.anthropic.com"); got != "blocks" {
		t.Fatalf("anthropic: %s", got)
	}
	if got := resolvePromptCacheHeuristic("gpt-4o", "https://api.openai.com/v1"); got != "key" {
		t.Fatalf("openai: %s", got)
	}
	if got := resolvePromptCacheHeuristic("qwen2.5", "http://127.0.0.1:8000/v1"); got != "off" {
		t.Fatalf("local: %s", got)
	}
	if got := resolvePromptCacheHeuristic("anthropic/claude-3.5-sonnet", "https://openrouter.ai/api/v1"); got != "blocks" {
		t.Fatalf("openrouter claude: %s", got)
	}
}

func TestIsPromptCacheRejectBody(t *testing.T) {
	if !isPromptCacheRejectBody(`unknown field "cache_control"`) {
		t.Fatal("cache_control")
	}
	if !isPromptCacheRejectBody("Extra inputs are not permitted: prompt_cache_key") {
		t.Fatal("prompt_cache_key")
	}
	if isPromptCacheRejectBody("model not found") {
		t.Fatal("unrelated")
	}
}

func TestProbeLLMPromptCacheBlocksOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		if len(msgs) == 0 {
			t.Fatal("no messages")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
		})
	}))
	defer srv.Close()

	res := probeLLMPromptCache(context.Background(), srv.URL, "sk-test", "claude-3-5-sonnet")
	if !res.OK || !res.SupportsPromptCache || res.Mode != "blocks" {
		t.Fatalf("%+v", res)
	}
}

func TestProbeLLMPromptCacheKeyOK(t *testing.T) {
	var sawKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["prompt_cache_key"]; ok {
			sawKey = true
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
		})
	}))
	defer srv.Close()

	res := probeLLMPromptCache(context.Background(), srv.URL, "sk-test", "gpt-4o-mini")
	if !res.OK || !res.SupportsPromptCache || res.Mode != "key" || !sawKey {
		t.Fatalf("%+v sawKey=%v", res, sawKey)
	}
}

func TestProbeLLMPromptCacheReject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["prompt_cache_key"]; ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"Extra inputs are not permitted: prompt_cache_key"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
		})
	}))
	defer srv.Close()

	res := probeLLMPromptCache(context.Background(), srv.URL, "sk-test", "gpt-4o")
	if !res.OK || res.SupportsPromptCache || res.Mode != "none" {
		t.Fatalf("%+v", res)
	}
	if !strings.Contains(res.Detail, "拒绝") {
		t.Fatalf("detail: %s", res.Detail)
	}
}

func TestProbeLLMPromptCacheHeuristicOff(t *testing.T) {
	// Local/compat models should not hit upstream when heuristic is off.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probeLLMPromptCache(context.Background(), srv.URL, "sk-test", "qwen2.5-7b")
	if !res.OK || res.SupportsPromptCache || res.Mode != "none" || called {
		t.Fatalf("%+v called=%v", res, called)
	}
}
