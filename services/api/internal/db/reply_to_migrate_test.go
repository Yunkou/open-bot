package db

import (
	"strings"
	"testing"
)

func TestShouldClearAssistantAutoReplyTo(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                          string
		role, replyTo, thread, parent string
		want                          bool
	}{
		{"auto bot quote", "assistant", "user-1", "", "user", true},
		{"auto bot quote whitespace thread", "assistant", "user-1", "  ", "user", true},
		{"explicit thread bot (keep)", "assistant", "user-1", "root-1", "user", false},
		{"user quote row", "user", "msg-1", "", "assistant", false},
		{"user quote with thread", "user", "msg-1", "msg-1", "assistant", false},
		{"assistant reply to assistant", "assistant", "asst-1", "", "assistant", false},
		{"empty reply_to", "assistant", "", "", "user", false},
		{"nil-like reply_to", "assistant", "  ", "", "user", false},
		{"summary role", "summary", "user-1", "", "user", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldClearAssistantAutoReplyTo(tc.role, tc.replyTo, tc.thread, tc.parent)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestClearAssistantAutoReplyToSQLShape(t *testing.T) {
	t.Parallel()
	sql := clearAssistantAutoReplyToSQL
	for _, needle := range []string{
		"role = 'assistant'",
		"reply_to_id = NULL",
		"thread_root_id IS NULL",
		"u.role = 'user'",
		"u.id = a.reply_to_id",
		"u.conversation_id = a.conversation_id",
	} {
		if !strings.Contains(sql, needle) {
			t.Fatalf("SQL missing %q", needle)
		}
	}
	// Must not wipe user-authored quotes.
	if strings.Contains(sql, "a.role = 'user'") {
		t.Fatal("must not UPDATE user rows")
	}
}

func TestClearAssistantAutoReplyToV1Marker(t *testing.T) {
	t.Parallel()
	if clearAssistantAutoReplyToV1Marker != "clear_assistant_auto_reply_to_v1" {
		t.Fatalf("marker %q", clearAssistantAutoReplyToV1Marker)
	}
}
