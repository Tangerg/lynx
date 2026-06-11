package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionConfig]; the defaults aim at "comfortably fits in
// 128k-context models" without doing per-turn token math.
const (
	defaultCompactThreshold  = 24 // total messages before we trigger
	defaultCompactKeepRecent = 6  // raw messages to preserve verbatim
)

// CompactionConfig tunes the auto-compaction heuristic.
//
// MaxMessages is the upper bound that triggers a compaction sweep.
// When the conversation grows past it, the oldest
// (len - KeepRecent) messages are replaced by a single system
// message carrying an LLM-generated summary.
//
// Zero values fall back to the package defaults.
type CompactionConfig struct {
	MaxMessages int // default: defaultCompactThreshold
	KeepRecent  int // default: defaultCompactKeepRecent
}

// Compactor is the auto-compaction worker. Constructed by the engine
// unless compaction is disabled (negative MaxMessages); a nil
// Compactor makes [Compactor.MaybeCompact] a silent no-op.
type Compactor struct {
	store      memory.Store
	client     *chat.Client
	threshold  int
	keepRecent int
}

// NewCompactor builds a Compactor over the chat-memory store and chat
// client. Zero / out-of-range config fields fall back to the package
// defaults.
func NewCompactor(store memory.Store, client *chat.Client, cfg CompactionConfig) *Compactor {
	threshold := cfg.MaxMessages
	if threshold <= 0 {
		threshold = defaultCompactThreshold
	}
	keep := cfg.KeepRecent
	if keep <= 0 {
		keep = defaultCompactKeepRecent
	}
	// Sanity: keep must be < threshold or compaction would loop on
	// the same message set.
	if keep >= threshold {
		keep = threshold / 2
	}
	return &Compactor{
		store:      store,
		client:     client,
		threshold:  threshold,
		keepRecent: keep,
	}
}

// MaybeCompact inspects sessionID's history. When the message
// count exceeds the threshold the older slice is summarized and
// the store is rewritten as [summary, recent...]. The returned
// [engine.CompactionResult] reports whether the sweep fired and the
// before/after message counts so callers can both chain follow-on
// work (e.g. extraction) and surface an observable boundary event.
//
// No-op (zero result) on a nil receiver (compaction disabled) or an
// empty sessionID.
//
// Important: the summary call goes through chat.Client directly
// (no middleware), so it does NOT enter the chat-memory middleware
// — otherwise the summarisation request itself would be appended
// to the history and trigger another compaction round.
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string) (engine.CompactionResult, error) {
	if c == nil || sessionID == "" {
		return engine.CompactionResult{}, nil
	}
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return engine.CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	if len(msgs) < c.threshold {
		return engine.CompactionResult{}, nil
	}

	before := len(msgs)
	cutoff := len(msgs) - c.keepRecent

	// Advance cutoff to the next UserMessage boundary so that `recent`
	// never starts mid-turn. Without this, the split can leave a
	// ToolMessage at the head of `recent` whose preceding AssistantMessage
	// (with tool_calls) ended up in `older` — producing an invalid
	// conversation where a tool result has no preceding tool_call, which
	// DeepSeek (and other strict providers) reject with 400.
	for cutoff < len(msgs) {
		if _, ok := msgs[cutoff].(*chat.UserMessage); ok {
			break
		}
		cutoff++
	}
	if cutoff >= len(msgs) {
		// No clean UserMessage boundary in the trailing segment —
		// skip this compaction cycle rather than corrupt the history.
		return engine.CompactionResult{}, nil
	}

	older := msgs[:cutoff]
	recent := msgs[cutoff:]

	summary, err := c.summarize(ctx, older)
	if err != nil {
		return engine.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	if err := c.store.Clear(ctx, sessionID); err != nil {
		return engine.CompactionResult{}, fmt.Errorf("compactor: clear: %w", err)
	}
	rewritten := make([]chat.Message, 0, 1+len(recent))
	rewritten = append(rewritten, summary)
	rewritten = append(rewritten, recent...)
	if err := c.store.Write(ctx, sessionID, rewritten...); err != nil {
		return engine.CompactionResult{}, err
	}
	return engine.CompactionResult{
		Compacted:      true,
		MessagesBefore: before,
		MessagesAfter:  len(rewritten),
	}, nil
}

// summarize asks the LLM to fold the older messages into a single
// system message of bullet points. Failure aborts compaction —
// keeping the existing history is always preferable to losing it
// behind a bad summary.
func (c *Compactor) summarize(ctx context.Context, msgs []chat.Message) (chat.Message, error) {
	transcript := renderTranscript(msgs)
	const prompt = `You are summarizing the earlier portion of a coding-agent
conversation that has grown too long to keep verbatim. Produce a
compact, faithful summary the agent can use to continue without
losing key context.

Format the summary as markdown bullets. Cover:
- Tasks completed
- Files / paths touched
- Open questions or remaining work
- User preferences / constraints stated
- Tool invocations of lasting relevance

Be specific and concise. Quote literal identifiers (file names,
function names) so they remain greppable. Do NOT include the
preamble or the user message; the agent will receive your bullets
verbatim as part of its system prompt.`

	text, err := askDirect(ctx, c.client, prompt, transcript)
	if err != nil {
		return nil, err
	}

	body := "[Earlier conversation summary]\n" + strings.TrimSpace(text)
	return chat.NewSystemMessage(body), nil
}
