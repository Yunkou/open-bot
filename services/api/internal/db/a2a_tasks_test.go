package db

import "testing"

func TestNormalizeA2AState(t *testing.T) {
	cases := map[string]string{
		"submitted":            A2AStateSubmitted,
		"TASK_STATE_WORKING":   A2AStateWorking,
		"completed":            A2AStateCompleted,
		"task_state_failed":    A2AStateFailed,
		"cancelled":            A2AStateCanceled,
		"TASK_STATE_CANCELED":  A2AStateCanceled,
	}
	for in, want := range cases {
		if got := NormalizeA2AState(in); got != want {
			t.Fatalf("%q -> %q want %q", in, got, want)
		}
	}
}

func TestA2AStateTerminal(t *testing.T) {
	if A2AStateTerminal(A2AStateWorking) {
		t.Fatal("working must not be terminal")
	}
	if !A2AStateTerminal(A2AStateCompleted) {
		t.Fatal("completed must be terminal")
	}
	if !A2AStateTerminal("cancelled") {
		t.Fatal("cancelled alias must be terminal")
	}
}

func TestA2ATaskPublicMap(t *testing.T) {
	task := &A2ATask{
		ID:            "t1",
		ContextID:     "c1",
		AgentID:       "bot",
		State:         A2AStateCompleted,
		ResultText:    "hi",
		ArtifactsJSON: `[{"name":"reply"}]`,
		HistoryJSON:   `[]`,
	}
	m := A2ATaskPublicMap(task)
	if m["id"] != "t1" || m["contextId"] != "c1" {
		t.Fatalf("ids: %#v", m)
	}
	status, _ := m["status"].(map[string]any)
	if status["state"] != A2AStateCompleted {
		t.Fatalf("state: %#v", status)
	}
	if m["result_text"] != "hi" {
		t.Fatalf("result_text missing")
	}
}
