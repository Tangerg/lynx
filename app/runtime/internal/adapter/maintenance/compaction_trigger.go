package maintenance

import "github.com/Tangerg/lynx/core/model/chat"

const (
	// charsPerToken is the coarse chars→tokens divisor used ONLY for the
	// compaction trigger estimate — never for billing. ~4 chars/token is
	// the usual English rule of thumb; mature agent runtimes drive this
	// decision with a similar cheap heuristic rather than
	// paying for real tokenization every turn boundary.
	charsPerToken = 4
)

// shouldCompact reports whether msgs has grown past either trigger: the
// raw message count or the estimated token footprint. The token estimate
// (see [estimateTokens]) is what catches a short conversation bloated by
// large tool results, which the message count alone misses.
func (c *Compactor) shouldCompact(msgs []chat.Message) bool {
	if len(msgs) >= c.maxMessages {
		return true
	}
	return estimateTokens(msgs) >= c.maxTokens
}

// estimateTokens approximates the token footprint of msgs from the
// flattened transcript length ([charsPerToken]). Deliberately coarse — it
// drives only the compaction trigger, never billing — and reuses
// [renderTranscript] so tool-result bodies (the bulk of a coding
// conversation) are counted, not just chat text.
func estimateTokens(msgs []chat.Message) int {
	return len(renderTranscript(msgs, uncappedToolResults)) / charsPerToken
}
