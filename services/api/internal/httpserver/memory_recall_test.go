package httpserver

import (
	"testing"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestParseRecallItemsAndStripMeta(t *testing.T) {
	items := parseRecallItems([]any{
		map[string]any{
			"source":  "explicit",
			"scope":   "bot",
			"tier":    "profile",
			"content": "likes tea",
			"snippet": "[bot] [profile] likes tea",
		},
		map[string]any{
			"source": "mem0",
			"scope":  "user",
			"content": "lives in shanghai",
			"score":  0.5,
		},
	})
	if len(items) != 2 {
		t.Fatalf("items=%d", len(items))
	}
	if items[0].Source != "explicit" || items[1].Score == nil || *items[1].Score != 0.5 {
		t.Fatalf("items=%+v", items)
	}

	// Persist without db should no-panic and strip key.
	s := &Server{}
	payload := map[string]any{
		"memory_recalled": 1,
		"memory_recall": map[string]any{
			"scene":          "dm",
			"explicit_count": 1,
			"mem0_count":     0,
			"items": []any{
				map[string]any{"source": "explicit", "content": "x"},
			},
		},
	}
	s.persistMemoryRecallFromMeta(payload, recallPersistContext{UserID: "u1", Source: "chat"})
	if _, ok := payload["memory_recall"]; ok {
		t.Fatal("memory_recall should be stripped even when db is nil")
	}
	_ = db.MemoryRecallItem{}
}
