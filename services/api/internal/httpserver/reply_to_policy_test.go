package httpserver

import "testing"

func TestBotAssistantMessageOptsOmitsReplyTo(t *testing.T) {
	opts := botAssistantMessageOpts("agent-1", "root-1", "run-abc")
	if opts.ReplyToID != "" {
		t.Fatalf("ReplyToID must be empty for Bot assistant rows, got %q", opts.ReplyToID)
	}
	if opts.AgentID != "agent-1" {
		t.Fatalf("AgentID: got %q", opts.AgentID)
	}
	if opts.ThreadRootID != "root-1" {
		t.Fatalf("ThreadRootID: got %q", opts.ThreadRootID)
	}
	if opts.RequestID != "run-abc" {
		t.Fatalf("RequestID: got %q", opts.RequestID)
	}

	empty := botAssistantMessageOpts("a", "  ", "  ")
	if empty.ThreadRootID != "" || empty.RequestID != "" {
		t.Fatalf("expected trimmed empty thread/request, got %+v", empty)
	}
}
