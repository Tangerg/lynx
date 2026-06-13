package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/lyra/internal/kernel"
)

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionConfig]. Two independent triggers, OR-composed: a raw
// message count and an estimated token footprint. The token trigger
// catches a conversation whose few messages carry large tool outputs (a
// single file read can outweigh twenty short turns) — which a message
// count alone misses. The defaults aim at "comfortably fits in
// 128k-context models".
const (
	defaultCompactMaxMessages = 24      // message count before we trigger
	defaultCompactKeepRecent  = 6       // raw messages to preserve verbatim
	defaultCompactMaxTokens   = 100_000 // estimated token footprint before we trigger

	// charsPerToken is the coarse chars→tokens divisor used ONLY for the
	// compaction trigger estimate — never for billing. ~4 chars/token is
	// the usual English rule of thumb; the field (Crush / harness9 / pi)
	// all drive this decision with a similar cheap heuristic rather than
	// paying for real tokenization every turn boundary.
	charsPerToken = 4
)

// CompactionConfig tunes the auto-compaction heuristic.
//
// A sweep triggers when EITHER bound is breached: MaxMessages (raw
// message count) or MaxTokens (estimated token footprint). On a sweep the
// oldest (len - KeepRecent) messages are replaced by a single system
// message carrying an LLM-generated summary.
//
// Zero values fall back to the package defaults.
type CompactionConfig struct {
	MaxMessages int // default: defaultCompactMaxMessages
	MaxTokens   int // default: defaultCompactMaxTokens (estimated footprint)
	KeepRecent  int // default: defaultCompactKeepRecent
}

// Compactor is the auto-compaction worker. Constructed by the engine
// unless compaction is disabled (negative MaxMessages); a nil
// Compactor makes [Compactor.MaybeCompact] a silent no-op.
type Compactor struct {
	store       memory.Store
	client      *chat.Client
	maxMessages int
	maxTokens   int
	keepRecent  int
}

// NewCompactor builds a Compactor over the chat-memory store and chat
// client. Zero / out-of-range config fields fall back to the package
// defaults.
func NewCompactor(store memory.Store, client *chat.Client, cfg CompactionConfig) *Compactor {
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = defaultCompactMaxMessages
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultCompactMaxTokens
	}
	keep := cfg.KeepRecent
	if keep <= 0 {
		keep = defaultCompactKeepRecent
	}
	// Sanity: keep must be < maxMessages or compaction would loop on
	// the same message set.
	if keep >= maxMessages {
		keep = maxMessages / 2
	}
	return &Compactor{
		store:       store,
		client:      client,
		maxMessages: maxMessages,
		maxTokens:   maxTokens,
		keepRecent:  keep,
	}
}

// MaybeCompact inspects sessionID's history. When either trigger
// (message count or estimated token footprint, see [shouldCompact]) is
// breached the older slice is summarized and the store is rewritten as
// [summary, recent...]. The returned
// [kernel.CompactionResult] reports whether the sweep fired and the
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
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string) (kernel.CompactionResult, error) {
	if c == nil || sessionID == "" {
		return kernel.CompactionResult{}, nil
	}
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return kernel.CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	if !c.shouldCompact(msgs) {
		return kernel.CompactionResult{}, nil
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
		return kernel.CompactionResult{}, nil
	}

	older := msgs[:cutoff]
	recent := msgs[cutoff:]

	summary, err := c.summarize(ctx, older)
	if err != nil {
		return kernel.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	if err := c.store.Clear(ctx, sessionID); err != nil {
		return kernel.CompactionResult{}, fmt.Errorf("compactor: clear: %w", err)
	}
	rewritten := make([]chat.Message, 0, 1+len(recent))
	rewritten = append(rewritten, summary)
	rewritten = append(rewritten, recent...)
	if err := c.store.Write(ctx, sessionID, rewritten...); err != nil {
		return kernel.CompactionResult{}, err
	}
	return kernel.CompactionResult{
		Compacted:      true,
		MessagesBefore: before,
		MessagesAfter:  len(rewritten),
	}, nil
}

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
	return len(renderTranscript(msgs)) / charsPerToken
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
