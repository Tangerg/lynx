package maintenance

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// compactionStore is the worker's narrow history view. Replace must be atomic:
// compaction rewrites the complete model context and cannot tolerate a
// clear-then-append fallback.
type compactionStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	Replace(ctx context.Context, sessionID string, messages ...chat.Message) error
}

// Compactor is the auto-compaction worker. Constructed by the kernel
// unless compaction is disabled (negative MaxMessages); a nil
// Compactor makes [Compactor.MaybeCompact] a silent no-op.
type Compactor struct {
	store             compactionStore
	client            ClientFunc
	liveState         LiveStateFunc // nil = no post-compaction live-state reminder
	maxMessages       int
	explicitMaxTokens int // cfg.MaxTokens override; 0 = derive from the run's model window
	fallbackWindow    int // default model's context window; used when the run's window is unknown
	keepRecent        int
}

// NewCompactor builds a Compactor over the chat history store and a
// per-call chat-client resolver. liveState (nil to disable) snapshots a
// session's still-active execution state so an LLM summary rung can remind the
// model of running shells / in-progress tasks the summary would otherwise drop.
// Zero / out-of-range config fields fall back to the package defaults.
func NewCompactor(store compactionStore, client ClientFunc, liveState LiveStateFunc, cfg CompactionConfig) *Compactor {
	maxMessages := cfg.MaxMessages
	if maxMessages <= 0 {
		maxMessages = defaultCompactMaxMessages
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
		store:             store,
		client:            client,
		liveState:         liveState,
		maxMessages:       maxMessages,
		explicitMaxTokens: cfg.MaxTokens,
		fallbackWindow:    cfg.ContextWindow,
		keepRecent:        keep,
	}
}

// tokenTrigger resolves the token-footprint compaction threshold for a run whose
// model has contextWindow tokens (0 = unknown). An explicit MaxTokens config wins;
// otherwise the trigger is window-relative to the RUN's model when known, else the
// default model's window, else a coarse fixed fallback. Resolving this per run
// (not once at construction) is what lets a run pinning a smaller-context model
// than the default still compact before it overflows that model's window.
func (c *Compactor) tokenTrigger(contextWindow int) int {
	if c.explicitMaxTokens > 0 {
		return c.explicitMaxTokens
	}
	window := contextWindow
	if window <= 0 {
		window = c.fallbackWindow
	}
	if window > 0 {
		return window * windowTriggerPct / 100
	}
	return defaultCompactMaxTokens
}

// MaybeCompact inspects sessionID's history. When either trigger
// (message count or estimated token footprint, see [shouldCompact]) is
// breached it runs a ladder, cheapest rung first: a non-LLM trim of oversized
// tool-call arguments and old tool-result bodies (see [Compactor.trimForBudget]);
// only if that leaves the footprint over budget is the older slice summarized by
// the LLM and the store rewritten as [summary, recent...]. A trim that suffices
// on its own rewrites history silently and reports no boundary (Compacted stays
// false) — it drops no messages. The returned [turn.CompactionResult] reports
// whether the LLM summary fired and the before/after message counts so callers
// can chain follow-on work (e.g. extraction) and surface an observable boundary
// event.
//
// No-op (zero result) on a nil receiver (compaction disabled) or an
// empty sessionID.
//
// Important: the summary call goes through chatclient.Client directly
// (no middleware), so it does NOT enter the chat history middleware
// — otherwise the summarisation request itself would be appended
// to the history and trigger another compaction round.
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string, contextWindow int, preCompact func(context.Context) bool) (turn.CompactionResult, error) {
	if c == nil || sessionID == "" {
		return turn.CompactionResult{}, nil
	}
	maxTokens := c.tokenTrigger(contextWindow)
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return turn.CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	if !c.shouldCompact(msgs, maxTokens) {
		return turn.CompactionResult{}, nil
	}
	// The whole history is within keep-recent — nothing OLDER to summarize, and
	// computing a cutoff here would go negative (len-keepRecent < 0 → an
	// out-of-range index in the boundary scan below). This is reachable: the
	// token-footprint trigger ([shouldCompact]) fires on a SHORT conversation
	// bloated by a few large tool results, where len(msgs) < keepRecent. Skip —
	// you can't compact messages you must keep.
	if len(msgs) <= c.keepRecent {
		return turn.CompactionResult{}, nil
	}

	// PreCompact hook gate: compaction is now committed (triggers + guards
	// passed), so this fires exactly when a compaction would run — a hook may
	// veto it. nil = always proceed.
	if preCompact != nil && !preCompact(ctx) {
		return turn.CompactionResult{}, nil
	}

	before := len(msgs)

	// Compaction ladder, cheapest rung first: replace oversized tool-call
	// arguments and OLD tool-result bodies with previews (deterministic, no LLM).
	// If that alone brings the footprint under budget, commit the trimmed history
	// and skip the LLM summary. This drops no messages and is invisible to the UI
	// transcript (which renders the full tool results) — it only slims the LLM's
	// re-sent context — so it reports no compaction boundary (Compacted stays
	// false) and, correctly, triggers no fact-extraction LLM call.
	trimmed, changed := c.trimForBudget(msgs)
	if changed && !c.shouldCompact(trimmed, maxTokens) {
		if err := c.store.Replace(ctx, sessionID, trimmed...); err != nil {
			return turn.CompactionResult{}, fmt.Errorf("compactor: replace trimmed: %w", err)
		}
		return turn.CompactionResult{}, nil
	}

	// Still over budget (or nothing was trimmable): fall to the LLM summary rung
	// over the ORIGINAL older slice — the summariser caps tool bodies for its own
	// input ([summaryToolResultCap]), so it sees more than the stored trim would
	// leave and produces a fuller summary.
	cutoff := len(msgs) - c.keepRecent

	// Advance cutoff to the next UserMessage boundary so that `recent`
	// never starts mid-turn. Without this, the split can leave a
	// ToolMessage at the head of `recent` whose preceding AssistantMessage
	// (with tool_calls) ended up in `older` — producing an invalid
	// conversation where a tool result has no preceding tool_call, which
	// DeepSeek (and other strict providers) reject with 400.
	for cutoff < len(msgs) {
		if msgs[cutoff].Role == chat.RoleUser {
			break
		}
		cutoff++
	}
	if cutoff >= len(msgs) {
		// No clean UserMessage boundary in the trailing segment —
		// skip this compaction cycle rather than corrupt the history.
		return turn.CompactionResult{}, nil
	}

	older := msgs[:cutoff]
	recent := msgs[cutoff:]

	summary, err := c.summarize(ctx, older)
	if err != nil {
		return turn.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	rewritten := make([]chat.Message, 0, 2+len(recent))
	rewritten = append(rewritten, summary)
	// Right after the summary, carry over the live execution state the summary
	// dropped (running background shells, in-progress tasks) so the model does not
	// forget a job it started before the compacted turns. Deterministic, no model
	// call; omitted entirely when nothing is active.
	if c.liveState != nil {
		if reminder, ok := liveStateReminder(c.liveState(ctx, sessionID)); ok {
			rewritten = append(rewritten, reminder)
		}
	}
	rewritten = append(rewritten, recent...)
	// Atomically swap the history for [summary, ...recent]. The store rolls back
	// a failed rewrite, so a crash cannot
	// leave the conversation cleared-but-not-rewritten (losing `recent` too).
	if err := c.store.Replace(ctx, sessionID, rewritten...); err != nil {
		return turn.CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
	}
	return turn.CompactionResult{
		Compacted:      true,
		MessagesBefore: before,
		MessagesAfter:  len(rewritten),
	}, nil
}
