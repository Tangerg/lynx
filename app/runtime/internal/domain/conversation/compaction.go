package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

// Compaction is one complete coordinate transformation of a conversation.
// The prefix [0, Cutoff) is represented after the rewrite by PrefixAfter
// messages (normally a summary plus an optional live-state reminder); the
// suffix keeps its relative positions. Historical watermarks inside the folded
// prefix consequently collapse onto the one boundary the compacted history can
// still express.
type Compaction struct {
	expectedCount int
	cutoff        int
	prefixAfter   int
	replacement   Conversation
}

// NewCompaction validates a complete compaction replacement. A zero cutoff and
// prefix describe a content-only rewrite such as tool-result trimming: message
// coordinates stay unchanged.
func NewCompaction(expectedCount, cutoff, prefixAfter int, messages []chat.Message) (Compaction, error) {
	switch {
	case expectedCount < 0:
		return Compaction{}, errors.New("conversation: compaction expected count must not be negative")
	case cutoff < 0 || cutoff > expectedCount:
		return Compaction{}, fmt.Errorf("conversation: compaction cutoff %d is outside [0,%d]", cutoff, expectedCount)
	case prefixAfter < 0:
		return Compaction{}, errors.New("conversation: compaction replacement prefix must not be negative")
	case cutoff == 0 && prefixAfter != 0:
		return Compaction{}, errors.New("conversation: content-only compaction cannot introduce a replacement prefix")
	case cutoff > 0 && prefixAfter == 0:
		return Compaction{}, errors.New("conversation: folded compaction requires a replacement prefix")
	}
	replacement, err := New(messages)
	if err != nil {
		return Compaction{}, err
	}
	wantCount := prefixAfter + expectedCount - cutoff
	if replacement.Count() != wantCount {
		return Compaction{}, fmt.Errorf(
			"conversation: compaction replacement has %d messages, want %d",
			replacement.Count(), wantCount,
		)
	}
	return Compaction{
		expectedCount: expectedCount,
		cutoff:        cutoff,
		prefixAfter:   prefixAfter,
		replacement:   replacement,
	}, nil
}

// RebaseMessageMark maps an old conversation watermark into the replacement's
// coordinate space. Zero remains the exact boundary before all conversation
// content. Positive marks inside the summarized prefix coalesce at its new end;
// marks in the retained suffix preserve their distance from the cut.
func (c Compaction) RebaseMessageMark(mark int) (int, error) {
	if mark < 0 || mark > c.expectedCount {
		return 0, fmt.Errorf(
			"conversation: message watermark %d is outside [0,%d]",
			mark, c.expectedCount,
		)
	}
	if mark == 0 || c.cutoff == 0 {
		return mark, nil
	}
	if mark <= c.cutoff {
		return c.prefixAfter, nil
	}
	return c.prefixAfter + mark - c.cutoff, nil
}

func (c Compaction) ExpectedCount() int       { return c.expectedCount }
func (c Compaction) Cutoff() int              { return c.cutoff }
func (c Compaction) ReplacementPrefix() int   { return c.prefixAfter }
func (c Compaction) Messages() []chat.Message { return c.replacement.Messages() }
