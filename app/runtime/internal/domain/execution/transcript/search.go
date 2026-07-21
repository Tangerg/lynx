package transcript

import (
	"strings"
	"time"
)

// SearchHit is one full-text transcript search result: the conversation item
// that matched, keyed for provenance, with a snippet of the matching text.
type SearchHit struct {
	SessionID string
	RunID     string
	ItemID    string
	Kind      ItemKind
	CreatedAt time.Time
	Snippet   string
}

// SearchableText returns the conversation text of item to full-text index and
// whether item is one whose text belongs in session search. Only the
// human-readable conversation turns — user and agent messages — are indexed;
// reasoning, plans, tool calls, questions, and compaction summaries are noise
// for a "did we discuss X" recall over past sessions. The text is drawn from
// the item's content blocks, where message text lives (the runs reducer stores
// finished user/agent messages as TextContent blocks, not the Text field).
func SearchableText(item Item) (string, bool) {
	switch item.Kind {
	case UserMessage, AgentMessage:
	default:
		return "", false
	}
	var b strings.Builder
	for _, block := range item.Content {
		if block.Kind != TextContent {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	if b.Len() == 0 {
		return "", false
	}
	return b.String(), true
}
