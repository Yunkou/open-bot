package httpserver

import "testing"

func TestResolvePersistedThreadRootID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		client string
		reply  string
		want   string
	}{
		{"mainline plain", "", "", ""},
		{"mainline quote only", "", "parent-1", ""},
		{"mainline quote whitespace client", "  ", "parent-1", ""},
		{"sidebar explicit", "root-1", "", "root-1"},
		{"sidebar explicit with quote", "root-1", "parent-1", "root-1"},
		{"trim client", "  root-2  ", "x", "root-2"},
		// Must NOT behave like ResolveThreadRoot(parent) which would return reply id.
		{"never invent from reply_to", "", "would-have-been-root", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := resolvePersistedThreadRootID(tc.client, tc.reply)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestBotAssistantMessageOptsMainlineEmptyThread(t *testing.T) {
	t.Parallel()
	// Mainline quote turn: user has empty thread_root → Bot inherits empty.
	opts := botAssistantMessageOpts("agent-1", resolvePersistedThreadRootID("", "parent-1"), "run-1")
	if opts.ThreadRootID != "" {
		t.Fatalf("Bot on mainline quote must have empty thread_root, got %q", opts.ThreadRootID)
	}
	if opts.ReplyToID != "" {
		t.Fatalf("Bot ReplyToID must stay empty, got %q", opts.ReplyToID)
	}
	if opts.RequestID != "run-1" {
		t.Fatalf("RequestID: got %q", opts.RequestID)
	}
}

func TestBotAssistantMessageOptsSidebarKeepsThread(t *testing.T) {
	t.Parallel()
	root := resolvePersistedThreadRootID("root-9", "msg-in-thread")
	opts := botAssistantMessageOpts("agent-1", root, "run-2")
	if opts.ThreadRootID != "root-9" {
		t.Fatalf("sidebar Bot should keep thread_root, got %q", opts.ThreadRootID)
	}
}
