package runmaintenance

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/scope/app/runtime/internal/adapter/utilitymodel"
)

// compactionStore is the worker's narrow conversation use-case view. The
// implementation owns the cross-aggregate transaction that replaces history
// and rebases Run watermarks; this model worker only decides summary content.
type compactionStore interface {
	Read(ctx context.Context, sessionID string) ([]chat.Message, error)
	RewriteForCompaction(
		ctx context.Context,
		sessionID string,
		expectedCount int,
		cutoff int,
		replacementPrefix int,
		messages ...chat.Message,
	) error
}

// Compactor is the automatic conversation-history compaction worker. A nil
// Compactor makes [Compactor.CompactIfNeeded] a silent no-op.
type Compactor struct {
	store             compactionStore
	client            utilitymodel.Resolver
	liveState         LiveStateSnapshotter // nil = no post-compaction live-state reminder
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
	required       bool
	messagesBefore int
	cutoff         int
	trimmed        []chat.Message
	older          []chat.Message
	recent         []chat.Message
}

// CompactionResult describes an LLM summary rewrite. A trim-only rewrite keeps
// every message and therefore returns the zero value: callers publish a
// compaction boundary only when older messages were replaced by a summary.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// NewCompactor builds a Compactor over the chat history store and a
// per-call chat-client resolver. liveState (nil to disable) snapshots a
// session's still-active process state so an LLM summary rung can remind the
// model of running shells the summary cannot reconstruct.
// Zero / out-of-range config fields fall back to the package defaults.
func NewCompactor(store compactionStore, client utilitymodel.Resolver, liveState LiveStateSnapshotter, cfg CompactionConfig) *Compactor {
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

// CompactIfNeeded inspects sessionID's history. When either trigger
// (message count or complete-request token footprint, see [modelContextBudget]) is
// breached it runs a ladder, cheapest rung first: a non-LLM trim of oversized
// tool-call arguments and old tool-result bodies (see trimForBudgetBefore);
// only if that leaves the footprint over budget is the older slice summarized by
// the LLM and the store rewritten as [summary, recent...]. A trim that suffices
// on its own rewrites history silently and reports no boundary (Compacted stays
// false) — it drops no messages. The returned [CompactionResult] reports
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
func (c *Compactor) CompactIfNeeded(ctx context.Context, sessionID string, contextWindow int, preCompact func(context.Context) bool) (CompactionResult, error) {
	if c == nil || sessionID == "" {
		return CompactionResult{}, nil
	}
	maxTokens := c.tokenTrigger(contextWindow)
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	budget := newModelContextBudget(c.maxMessages, maxTokens, nil, nil, nil, chat.Options{}, 0)
	plan, err := c.planCompaction(msgs, budget)
	if err != nil {
		return CompactionResult{}, err
	}
	if plan.action == noCompaction {
		return CompactionResult{}, nil
	}
	if preCompact != nil && !preCompact(ctx) {
		return CompactionResult{}, nil
	}

	if plan.action == trimCompaction {
		if rewriteForCompactionErr := c.store.RewriteForCompaction(ctx, sessionID, len(msgs), 0, 0, plan.trimmed...); rewriteForCompactionErr != nil {
			return CompactionResult{}, fmt.Errorf("compactor: replace trimmed: %w", rewriteForCompactionErr)
		}
		return CompactionResult{}, nil
	}

	summary, err := c.summarize(ctx, plan.older)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	rewritten := make([]chat.Message, 0, 2+len(plan.recent))
	rewritten = append(rewritten, summary)
	// Right after the summary, carry over the live execution state the summary
	// dropped (running background shells) so the model does not forget a process
	// it started before the compacted Runs. Deterministic, no model
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
	prefixAfter := len(rewritten) - len(plan.recent)
	if err := c.store.RewriteForCompaction(
		ctx, sessionID, plan.messagesBefore, plan.cutoff, prefixAfter, rewritten...,
	); err != nil {
		return CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
	}
	return CompactionResult{
		Compacted:      true,
		MessagesBefore: plan.messagesBefore,
		MessagesAfter:  len(rewritten),
	}, nil
}

func (c *Compactor) planCompaction(
	messages []chat.Message,
	budget modelContextBudget,
) (compactionPlan, error) {
	return c.planCompactionWithProtectedTail(messages, budget, 0)
}

func (c *Compactor) planCompactionWithProtectedTail(
	messages []chat.Message,
	budget modelContextBudget,
	protectedTail int,
) (compactionPlan, error) {
	overBudget, err := budget.triggered(messages)
	if err != nil {
		return compactionPlan{}, err
	}
	if !overBudget || len(messages) == 0 {
		return compactionPlan{}, nil
	}
	if protectedTail < 0 || protectedTail > len(messages) {
		return compactionPlan{required: true}, nil
	}
	foldableLimit := len(messages) - protectedTail
	protectedOverBudget, err := budget.exceeded(messages[foldableLimit:])
	if err != nil {
		return compactionPlan{}, err
	}
	if foldableLimit == 0 || protectedOverBudget {
		return compactionPlan{required: true}, nil
	}
	cutoff := c.summaryCutoffWithProtectedTail(messages, protectedTail)
	if cutoff == 0 {
		return compactionPlan{required: true}, nil
	}
	trimmed, changed := trimForBudgetBefore(messages, cutoff)
	trimmedOverBudget, err := budget.exceeded(trimmed)
	if err != nil {
		return compactionPlan{}, err
	}
	if changed && !trimmedOverBudget {
		return compactionPlan{
			action: trimCompaction, required: true,
			messagesBefore: len(messages), trimmed: trimmed,
		}, nil
	}
	// KeepRecent is a preference, not a license to preserve a suffix that is
	// already over budget by itself. In that case the preferred prefix summary
	// cannot converge: every later pass would keep the same oversized turn.
	// Widen the deterministic rung first; only summarize the complete, finished
	// history when cheap trimming still cannot make it executable.
	recentOverBudget, err := budget.exceeded(trimmed[cutoff:])
	if err != nil {
		return compactionPlan{}, err
	}
	if recentOverBudget {
		cutoff = foldableLimit
		trimmed, changed = trimForBudgetBefore(messages, cutoff)
		trimmedOverBudget, err = budget.exceeded(trimmed)
		if err != nil {
			return compactionPlan{}, err
		}
		if changed && !trimmedOverBudget {
			return compactionPlan{
				action: trimCompaction, required: true,
				messagesBefore: len(messages), trimmed: trimmed,
			}, nil
		}
	}
	return compactionPlan{
		action:         summarizeCompaction,
		required:       true,
		messagesBefore: len(messages),
		cutoff:         cutoff,
		older:          trimmed[:cutoff],
		recent:         trimmed[cutoff:],
	}, nil
}

// summaryCutoffWithProtectedTail returns a complete-turn boundary near the
// configured recent window without folding the caller-owned exact suffix. The
// preferred boundary is the first User message at or after the naive cutoff.
// If the cutoff landed inside the final foldable turn, it moves back to that
// turn's opening User message.
func (c *Compactor) summaryCutoffWithProtectedTail(
	messages []chat.Message,
	protectedTail int,
) int {
	if protectedTail < 0 || protectedTail > len(messages) {
		return 0
	}
	foldable := messages[:len(messages)-protectedTail]
	desired := max(0, len(messages)-c.keepRecent)
	desired = min(desired, len(foldable))
	hasOpeningUser := false
	for index := desired; index < len(foldable); index++ {
		if foldable[index].Role == chat.RoleUser {
			if index > 0 {
				return index
			}
			hasOpeningUser = true
		}
	}
	for index := min(desired-1, len(foldable)-1); index >= 0; index-- {
		if foldable[index].Role == chat.RoleUser {
			if index > 0 {
				return index
			}
			hasOpeningUser = true
			break
		}
	}
	if hasOpeningUser {
		return len(foldable)
	}
	return 0
}
