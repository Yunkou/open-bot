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
