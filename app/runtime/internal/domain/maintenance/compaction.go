package maintenance

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// compactionDefaults govern the auto-compact trigger. Tunable via
// [CompactionConfig]. Two independent triggers, OR-composed: a raw
// message count and an estimated token footprint. The token trigger
// catches a conversation whose few messages carry large tool outputs (a
// single file read can outweigh twenty short turns) — which a message
// count alone misses. The defaults aim at "comfortably fits in
// 128k-context models".
const (
	defaultCompactMaxMessages = 24 // message count trigger threshold
	defaultCompactKeepRecent  = 6  // raw messages to preserve verbatim

	// defaultCompactMaxTokens is the estimated-token-footprint trigger used
	// ONLY when the model's real context window is unknown (catalog miss). When
	// the window IS known the trigger is window-relative instead — see
	// [CompactionConfig.ContextWindow] / [windowTriggerPct].
	defaultCompactMaxTokens = 100_000

	// windowTriggerPct is the share of the model's context window at which an
	// estimated footprint triggers compaction — leaving headroom for the
	// summary output + the next turn. A fixed number is wrong across the 32k…1M
	// window range; a relative trigger tracks the actual model's context
	// window rather than a fixed number that's wrong at either extreme.
	windowTriggerPct = 80

	// charsPerToken is the coarse chars→tokens divisor used ONLY for the
	// compaction trigger estimate — never for billing. ~4 chars/token is
	// the usual English rule of thumb; mature agent runtimes drive this
	// decision with a similar cheap heuristic rather than
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
	MaxTokens   int // explicit token-footprint trigger; overrides the window-relative default
	KeepRecent  int // default: defaultCompactKeepRecent
	// ContextWindow is the model's context window in tokens (from the catalog).
	// When > 0 and MaxTokens is unset, the token trigger becomes
	// ContextWindow*windowTriggerPct% — relative to the real model instead of a
	// fixed number. 0 (catalog miss) falls back to defaultCompactMaxTokens.
	ContextWindow int
}

// Compactor is the auto-compaction worker. Constructed by the engine
// unless compaction is disabled (negative MaxMessages); a nil
// Compactor makes [Compactor.MaybeCompact] a silent no-op.
type Compactor struct {
	store       memory.Store
	client      ClientFunc
	maxMessages int
	maxTokens   int
	keepRecent  int
}

// NewCompactor builds a Compactor over the chat-memory store and a
// per-call chat-client resolver. Zero / out-of-range config fields fall back
// to the package defaults.
func NewCompactor(store memory.Store, client ClientFunc, cfg CompactionConfig) *Compactor {
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = defaultCompactMaxMessages
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		// Window-relative when the model's context window is known, else a
		// coarse fixed fallback. Relative tracks the real model across the
		// 32k…1M range; fixed was wrong at both ends.
		if cfg.ContextWindow > 0 {
			maxTokens = cfg.ContextWindow * windowTriggerPct / 100
		} else {
			maxTokens = defaultCompactMaxTokens
		}
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
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (kernel.CompactionResult, error) {
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
	// The whole history is within keep-recent — nothing OLDER to summarize, and
	// computing a cutoff here would go negative (len-keepRecent < 0 → an
	// out-of-range index in the boundary scan below). This is reachable: the
	// token-footprint trigger ([shouldCompact]) fires on a SHORT conversation
	// bloated by a few large tool results, where len(msgs) < keepRecent. Skip —
	// you can't compact messages you must keep.
	if len(msgs) <= c.keepRecent {
		return kernel.CompactionResult{}, nil
	}

	// PreCompact hook gate: compaction is now committed (triggers + guards
	// passed), so this fires exactly when a compaction would run — a hook may
	// veto it. nil = always proceed.
	if preCompact != nil && !preCompact(ctx) {
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

	rewritten := make([]chat.Message, 0, 1+len(recent))
	rewritten = append(rewritten, summary)
	rewritten = append(rewritten, recent...)
	// Atomically swap the history for [summary, ...recent] via memory.Replace —
	// a transactional backend rolls back on a failed rewrite, so a crash can't
	// leave the conversation cleared-but-not-rewritten (losing `recent` too).
	if err := memory.Replace(ctx, c.store, sessionID, rewritten...); err != nil {
		return kernel.CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
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
	const prompt = `You are compacting the earlier portion of a long coding-agent
conversation into a faithful, STRUCTURED summary the agent will read as part of
its system prompt to continue WITHOUT losing key context. Be specific; quote
literal identifiers (file paths, function / type names, commands) so they stay
greppable.

Output markdown under EXACTLY these headings (drop a heading only if truly empty):

## Goal
The user's original objective(s), in their own framing — quote the key request.

## Progress
What has been done so far: completed steps, what worked.

## Current state
Files / paths created or modified (with their paths) + each one's role; key
identifiers (functions, types, symbols) in play; command results worth keeping.

## Decisions & constraints
Choices made and WHY; user preferences / constraints stated (style, libraries,
dos & don'ts); approaches rejected and the reason (so they aren't retried).

## Next steps
Remaining work + open questions — concrete and ordered.

Do NOT echo this instruction or restate the raw transcript; the agent receives
your sections verbatim.`

	var client *chat.Client
	if c.client != nil {
		client = c.client(ctx)
	}
	text, err := askDirect(ctx, client, prompt, transcript)
	if err != nil {
		return nil, err
	}

	body := "[Earlier conversation summary]\n" + strings.TrimSpace(text)
	return chat.NewSystemMessage(body), nil
}
