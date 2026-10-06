package httpserver

import (
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

// replyQuoteMaxRunes caps the quoted parent text forwarded to the runtime
// (runtime truncates again to ~2000 chars; this keeps the payload small).
const replyQuoteMaxRunes = 2000

// resolveReplyQuote finds the explicit-reply parent in this conversation's
// messages and returns its (truncated) content. ok=false when replyToID is
// blank or the parent is not in msgs. Callers that received a non-empty
// reply_to_id must reject the request on ok=false (same-conversation check).
func resolveReplyQuote(msgs []db.Message, replyToID string) (string, bool) {
	replyToID = strings.TrimSpace(replyToID)
	if replyToID == "" {
		return "", false
	}
	for _, m := range msgs {
		if m.ID != replyToID {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			return "", false
		}
		if r := []rune(c); len(r) > replyQuoteMaxRunes {
			c = string(r[:replyQuoteMaxRunes]) + "…"
		}
		return c, true
	}
	return "", false
}
