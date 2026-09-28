package httpserver

import (
	"testing"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestParseMentionTokens(t *testing.T) {
	got := parseMentionTokens("hi @open-bot and @通用助手, also @open-bot again")
	if len(got) != 2 {
		t.Fatalf("expected 2 unique tokens, got %#v", got)
	}
}

func TestResolveMentionedAgents(t *testing.T) {
	members := []string{"open-bot", "general", "custom-1"}
	agents := []*db.Agent{
		{ID: "open-bot", Name: "open-bot"},
		{ID: "general", Name: "通用助手"},
		{ID: "custom-1", Name: "码农助手"},
	}
	got := resolveMentionedAgents([]string{"通用助手", "custom-1"}, members, agents)
	if len(got) != 2 || got[0] != "general" || got[1] != "custom-1" {
		t.Fatalf("unexpected %#v", got)
	}
	got2 := resolveMentionedAgents([]string{"Open-Bot"}, members, agents)
	if len(got2) != 1 || got2[0] != "open-bot" {
		t.Fatalf("id case fold failed: %#v", got2)
	}
}
