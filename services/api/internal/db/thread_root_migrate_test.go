package db

import (
	"strings"
	"testing"
)

func TestIsShallowAutoThreadRoot(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		userN, quoteN int
		want bool
	}{
		{"classic auto quote thread", 1, 1, true},
		{"continued sidebar (2 users)", 2, 1, false},
		{"continued sidebar (2 quote seeds — unusual)", 2, 2, false},
		{"sole user joined existing root", 1, 0, false},
		{"empty group", 0, 0, false},
		{"bots only (no user)", 0, 0, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isShallowAutoThreadRoot(tc.userN, tc.quoteN)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestUserMessageLooksLikeAutoThreadSeed(t *testing.T) {
	t.Parallel()
	if !userMessageLooksLikeAutoThreadSeed("parent-1", "parent-1") {
		t.Fatal("expected auto seed when reply_to == thread_root")
	}
	if userMessageLooksLikeAutoThreadSeed("other", "root-1") {
		t.Fatal("inherited root must not look like auto seed")
	}
	if userMessageLooksLikeAutoThreadSeed("", "root-1") {
		t.Fatal("empty reply_to is not an auto seed")
	}
	if userMessageLooksLikeAutoThreadSeed("parent-1", "") {
		t.Fatal("empty thread_root is not an auto seed")
	}
	if !userMessageLooksLikeAutoThreadSeed("  p  ", "p") {
		t.Fatal("trim should match")
	}
}

func TestClearAutoThreadRootSQLShape(t *testing.T) {
	t.Parallel()
	sql := clearAutoThreadRootSQL
	for _, needle := range []string{
		"auto_roots",
		"thread_root_id = NULL",
		"role = 'user'",
		"reply_to_id = thread_root_id",
		"COUNT(*) FILTER",
		"m.thread_root_id = a.thread_root_id",
		"m.conversation_id = a.conversation_id",
	} {
		if !strings.Contains(sql, needle) {
			t.Fatalf("SQL missing %q", needle)
		}
	}
	// Must not delete rows — only null thread_root.
	if strings.Contains(strings.ToUpper(sql), "DELETE ") {
		t.Fatal("must not DELETE messages")
	}
}

func TestClearAutoThreadRootV1Marker(t *testing.T) {
	t.Parallel()
	if clearAutoThreadRootV1Marker != "clear_auto_thread_root_v1" {
		t.Fatalf("marker %q", clearAutoThreadRootV1Marker)
	}
}
