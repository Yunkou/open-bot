package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsAutoToolChoiceUnsupportedBody(t *testing.T) {
	if !isAutoToolChoiceUnsupportedBody(`"auto" tool choice requires --enable-auto-tool-choice`) {
		t.Fatal("classic")
	}
	if isAutoToolChoiceUnsupportedBody("model not found") {
		t.Fatal("unrelated")
	}
}

func TestProbeLLMToolsNative(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "1",
								"type": "function",
								"function": map[string]any{
									"name":      "ping",
									"arguments": "{}",
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	res := probeLLMTools(context.Background(), srv.URL, "sk-test", "demo-model")
	if !res.OK || !res.CanEnableTools || res.Mode != "native" {
		t.Fatalf("%+v", res)
	}
}

func TestProbeLLMToolsForcedOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, hasTools := body["tools"]; hasTools {
			if body["tool_choice"] == nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"\"auto\" tool choice requires --enable-auto-tool-choice and --tool-call-parser to be set"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"tool_calls": []map[string]any{
								{"id": "1", "type": "function", "function": map[string]any{"name": "ping", "arguments": "{}"}},
							},
						},
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
		})
	}))
	defer srv.Close()

	res := probeLLMTools(context.Background(), srv.URL, "sk-test", "demo-model")
	if !res.OK || res.Mode != "forced_only" {
		t.Fatalf("%+v", res)
	}
}

func TestProbeLLMToolsNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, hasTools := body["tools"]; hasTools {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"tools not supported"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
		})
	}))
	defer srv.Close()

	res := probeLLMTools(context.Background(), srv.URL, "sk-test", "demo-model")
	if res.OK || res.CanEnableTools || res.Mode != "none" {
		t.Fatalf("%+v", res)
	}
}

func TestIsOpenAIReasoningModel(t *testing.T) {
	for _, m := range []string{"gpt-6-luna", "gpt-5.6-luna", "gpt-5-luna", "o3-mini", "gpt-5", "foo-thinking"} {
		if !isOpenAIReasoningModel(m) {
			t.Fatalf("expected reasoning: %s", m)
		}
	}
	for _, m := range []string{"gpt-4o", "MiniMax-M3.1-Flash-Preview", "Qwen3-32B", ""} {
		if isOpenAIReasoningModel(m) {
			t.Fatalf("expected non-reasoning: %s", m)
		}
	}
}

func TestProbeLLMToolsReasoningEffortNone(t *testing.T) {
	// Simulates luna / gpt-5 gateways: tools rejected unless reasoning_effort=none.
	var sawNone bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, hasTools := body["tools"]; !hasTools {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{"message": map[string]any{"content": "hi"}}},
			})
			return
		}
		if body["reasoning_effort"] != "none" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Function tools with reasoning_effort are not supported for gpt-6-luna in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'."}}`))
			return
		}
		sawNone = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{"id": "1", "type": "function", "function": map[string]any{"name": "ping", "arguments": "{}"}},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	// Heuristic path: model name triggers reasoning_effort=none on first try.
	res := probeLLMTools(context.Background(), srv.URL, "sk-test", "gpt-6-luna")
	if !res.OK || !res.CanEnableTools || res.Mode != "native" || !sawNone {
		t.Fatalf("heuristic luna: %+v sawNone=%v", res, sawNone)
	}

	// Retry path: opaque id that is not heuristically reasoning, but gateway demands none.
	sawNone = false
	res2 := probeLLMTools(context.Background(), srv.URL, "sk-test", "opaque-gateway-model")
	if !res2.OK || !res2.CanEnableTools || res2.Mode != "native" || !sawNone {
		t.Fatalf("retry opaque: %+v sawNone=%v", res2, sawNone)
	}
}
