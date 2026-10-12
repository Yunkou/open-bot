package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelsURLCandidates(t *testing.T) {
	got := modelsURLCandidates("https://api.openai.com/v1/")
	if len(got) != 1 || got[0] != "https://api.openai.com/v1/models" {
		t.Fatalf("%v", got)
	}
	got = modelsURLCandidates("http://localhost:8000")
	if len(got) != 2 || got[0] != "http://localhost:8000/models" || got[1] != "http://localhost:8000/v1/models" {
		t.Fatalf("%v", got)
	}
}

func TestParseOpenAIModelsBody(t *testing.T) {
	ids, err := parseOpenAIModelsBody([]byte(`{"data":[{"id":"b"},{"id":"a"},{"id":"a"},{"id":""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("%v", ids)
	}
}

func TestFetchLLMModelsOK(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-4o-mini"},
				{"id": "gpt-4o"},
			},
		})
	}))
	defer srv.Close()

	res := fetchLLMModels(context.Background(), srv.URL+"/v1", "sk-secret-should-not-leak")
	if !res.OK || len(res.Models) != 2 {
		t.Fatalf("%+v", res)
	}
	if sawAuth != "Bearer sk-secret-should-not-leak" {
		t.Fatalf("auth=%q", sawAuth)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), "sk-secret") {
		t.Fatalf("api key leaked in response: %s", blob)
	}
}

func TestFetchLLMModelsFallbackV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "local-model"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	res := fetchLLMModels(context.Background(), srv.URL, "sk-test")
	if !res.OK || len(res.Models) != 1 || res.Models[0] != "local-model" {
		t.Fatalf("%+v", res)
	}
}

func TestFetchLLMModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key"}}`))
	}))
	defer srv.Close()

	res := fetchLLMModels(context.Background(), srv.URL+"/v1", "sk-bad")
	if res.OK || res.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("%+v", res)
	}
	if strings.Contains(res.Error, "sk-bad") || strings.Contains(res.Detail, "sk-bad") {
		t.Fatalf("key in error: %+v", res)
	}
}

func TestFetchLLMModelsMissingArgs(t *testing.T) {
	res := fetchLLMModels(context.Background(), "", "k")
	if res.OK {
		t.Fatal(res)
	}
	res = fetchLLMModels(context.Background(), "http://x", "")
	if res.OK {
		t.Fatal(res)
	}
}
