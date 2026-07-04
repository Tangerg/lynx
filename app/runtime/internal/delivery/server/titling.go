package server

import (
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// userMessageText flattens a run's opening user input to plain text for the
// titler — text blocks joined by newlines; image blocks are ignored (a title
// comes from words, not pixels).
func userMessageText(blocks []protocol.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type != protocol.ContentBlockText || blk.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(blk.Text)
	}
	return strings.TrimSpace(b.String())
}
