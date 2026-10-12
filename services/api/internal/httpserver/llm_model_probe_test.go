package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeLLMModelOK(t *testing.T) {
	var sawAuth, sawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		sawBody = string(buf[:n])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	res := probeLLMModel(context.Background(), srv.URL+"/v1", "sk-secret-should-not-leak", "gpt-4o-mini")
	if !res.OK {
		t.Fatalf("%+v", res)
	}
	if res.Model != "gpt-4o-mini" {
		t.Fatalf("model=%q", res.Model)
	}
	if res.LatencyMs < 0 {
		t.Fatalf("latency=%d", res.LatencyMs)
	}
	if sawAuth != "Bearer sk-secret-should-not-leak" {
		t.Fatalf("auth=%q", sawAuth)
	}
	if !strings.Contains(sawBody, `"gpt-4o-mini"`) {
		t.Fatalf("body missing model: %s", sawBody)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "sk-secret") {
		t.Fatalf("api key leaked in response: %s", blob)
	}
	if !strings.Contains(res.Detail, "模型可用") {
		t.Fatalf("detail=%q", res.Detail)
	}
}

func TestProbeLLMModelHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"model_not_found"}}`))
	}))
	defer srv.Close()

	res := probeLLMModel(context.Background(), srv.URL+"/v1", "sk-test", "missing-model")
	if res.OK {
		t.Fatalf("expected failure: %+v", res)
	}
	if res.HTTPStatus != http.StatusNotFound {
		t.Fatalf("status=%d", res.HTTPStatus)
	}
	if !strings.Contains(res.Detail, "不可用") {
		t.Fatalf("detail=%q", res.Detail)
	}
}

func TestProbeLLMModelMissingFields(t *testing.T) {
	res := probeLLMModel(context.Background(), "", "sk", "m")
	if res.OK || !strings.Contains(res.Detail, "base_url") {
		t.Fatalf("%+v", res)
	}
	res = probeLLMModel(context.Background(), "http://x", "", "m")
	if res.OK || !strings.Contains(res.Detail, "api_key") {
		t.Fatalf("%+v", res)
	}
}
