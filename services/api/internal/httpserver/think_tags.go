package httpserver

import (
	"regexp"
	"strings"
)

// Reasoning blocks must not be stored or replayed as visible assistant text.
var (
	thinkBlock    = regexp.MustCompile(`(?is)<(?:think|thinking|redacted_thinking)\b[^>]*>.*?</(?:think|thinking|redacted_thinking)>`)
	thinkUnclosed = regexp.MustCompile(`(?is)<(?:think|thinking|redacted_thinking)\b[^>]*>.*$`)
	thinkPartial  = regexp.MustCompile(`(?i)<(?:think|thinking|redacted_thinking)\b[^>]*$`)
)

func stripThinkTags(text string) string {
	if text == "" {
		return text
	}
	cleaned := thinkBlock.ReplaceAllString(text, "")
	cleaned = thinkUnclosed.ReplaceAllString(cleaned, "")
	cleaned = thinkPartial.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}
