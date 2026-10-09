package httpserver

import "testing"

func TestExtractA2AText(t *testing.T) {
	text, agent, ctx := extractA2AText(map[string]any{
		"text":     "hello",
		"agent_id": "a1",
		"contextId": "c1",
	})
	if text != "hello" || agent != "a1" || ctx != "c1" {
		t.Fatalf("got %q %q %q", text, agent, ctx)
	}
	text, _, _ = extractA2AText(map[string]any{
		"message": map[string]any{
			"parts": []any{
				map[string]any{"kind": "text", "text": "p1"},
				map[string]any{"text": "p2"},
			},
		},
	})
	if text != "p1\np2" {
		t.Fatalf("parts: %q", text)
	}
}

func TestExtractA2ATaskID(t *testing.T) {
	if got := extractA2ATaskID(map[string]any{"id": "x"}); got != "x" {
		t.Fatal(got)
	}
	if got := extractA2ATaskID(map[string]any{"taskId": "y"}); got != "y" {
		t.Fatal(got)
	}
}

func TestUsageFromDonePayload(t *testing.T) {
	u := usageFromDonePayload(map[string]any{
		"usage": map[string]any{"prompt_tokens": 10.0, "completion_tokens": 5.0},
	})
	if u.PromptTokens != 10 || u.CompletionTokens != 5 || u.TotalTokens != 15 {
		t.Fatalf("%+v", u)
	}
}
