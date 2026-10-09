package httpserver

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestLessonDraftFromNegativeFeedbackUsesLabels(t *testing.T) {
	title, body, tags := lessonDraftFromFeedback(&db.MessageFeedback{
		Polarity: "negative",
		Reasons:  []string{"verbose", "not_followed"},
		Note:     "先给结论，再展开",
	})
	if !strings.HasPrefix(title, "改进：") || !strings.Contains(title, "太啰嗦") {
		t.Fatalf("title = %q", title)
	}
	if !strings.Contains(body, "原因：太啰嗦、没按要求做") || !strings.Contains(body, "用户希望：先给结论，再展开") {
		t.Fatalf("body = %q", body)
	}
	if len(tags) != 2 || tags[0] != "verbose" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestLessonDraftLimitsAreRuneCounts(t *testing.T) {
	long := strings.Repeat("很长的补充说明", 200)
	title, body, _ := lessonDraftFromFeedback(&db.MessageFeedback{
		Polarity: "negative",
		Reasons:  []string{strings.Repeat("自定义原因", 20)},
		Note:     long,
	})
	if n := utf8.RuneCountInString(title); n > 40 {
		t.Fatalf("title runes = %d", n)
	}
	if n := utf8.RuneCountInString(body); n > 500 {
		t.Fatalf("body runes = %d", n)
	}
}

func TestLessonDraftPositive(t *testing.T) {
	title, body, _ := lessonDraftFromFeedback(&db.MessageFeedback{Polarity: "positive"})
	if title != "保持有效做法" || !strings.Contains(body, "正面反馈") {
		t.Fatalf("title=%q body=%q", title, body)
	}
}
