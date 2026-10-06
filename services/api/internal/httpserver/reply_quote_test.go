package httpserver

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestResolveReplyQuote(t *testing.T) {
	msgs := []db.Message{
		{ID: "m1", Role: "assistant", Content: "  原始回答  "},
		{ID: "m2", Role: "user", Content: "这个怎么理解？"},
	}
	if _, ok := resolveReplyQuote(msgs, ""); ok {
		t.Fatal("blank id must not resolve")
	}
	if _, ok := resolveReplyQuote(msgs, "missing"); ok {
		t.Fatal("unknown parent must not resolve")
	}
	got, ok := resolveReplyQuote(msgs, " m1 ")
	if !ok || got != "原始回答" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	long := []db.Message{{ID: "x", Content: strings.Repeat("字", replyQuoteMaxRunes+10)}}
	got, ok = resolveReplyQuote(long, "x")
	if !ok || utf8.RuneCountInString(got) != replyQuoteMaxRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("truncation failed: runes=%d", utf8.RuneCountInString(got))
	}
}
