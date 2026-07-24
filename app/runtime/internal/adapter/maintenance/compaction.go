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

type compactionAction uint8

const (
	noCompaction compactionAction = iota
	trimCompaction
	summarizeCompaction
)

type compactionPlan struct {
	action         compactionAction
	messagesBefore int
	trimmed        []chat.Message
	older          []chat.Message
	recent         []chat.Message
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
	plan := c.planCompaction(msgs, maxTokens)
	if plan.action == noCompaction {
		return turn.CompactionResult{}, nil
	}
	if preCompact != nil && !preCompact(ctx) {
		return turn.CompactionResult{}, nil
	}

	if plan.action == trimCompaction {
		if err := c.store.Replace(ctx, sessionID, plan.trimmed...); err != nil {
			return turn.CompactionResult{}, fmt.Errorf("compactor: replace trimmed: %w", err)
		}
		return turn.CompactionResult{}, nil
	}

	summary, err := c.summarize(ctx, plan.older)
	if err != nil {
		return turn.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	rewritten := make([]chat.Message, 0, 2+len(plan.recent))
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
	rewritten = append(rewritten, plan.recent...)
	// Atomically swap the history for [summary, ...recent]. The store rolls back
	// a failed rewrite, so a crash cannot
	// leave the conversation cleared-but-not-rewritten (losing `recent` too).
	if err := c.store.Replace(ctx, sessionID, rewritten...); err != nil {
		return turn.CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
	}
	return turn.CompactionResult{
		Compacted:      true,
		MessagesBefore: plan.messagesBefore,
		MessagesAfter:  len(rewritten),
	}, nil
}

func (c *Compactor) planCompaction(messages []chat.Message, maxTokens int) compactionPlan {
	if !c.shouldCompact(messages, maxTokens) || len(messages) <= c.keepRecent {
		return compactionPlan{}
	}
	trimmed, changed := c.trimForBudget(messages)
	if changed && !c.shouldCompact(trimmed, maxTokens) {
		return compactionPlan{action: trimCompaction, trimmed: trimmed}
	}

	cutoff := len(messages) - c.keepRecent
	for cutoff < len(messages) && messages[cutoff].Role != chat.RoleUser {
		cutoff++
	}
	if cutoff == len(messages) {
		return compactionPlan{}
	}
	return compactionPlan{
		action:         summarizeCompaction,
		messagesBefore: len(messages),
		older:          messages[:cutoff],
		recent:         messages[cutoff:],
	}
}
