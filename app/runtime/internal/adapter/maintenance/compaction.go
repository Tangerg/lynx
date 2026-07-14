package maintenance

import (
	"context"
	"fmt"

	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// Compactor is the auto-compaction worker. Constructed by the kernel
// unless compaction is disabled (negative MaxMessages); a nil
// Compactor makes [Compactor.MaybeCompact] a silent no-op.
type Compactor struct {
	store       history.Store
	client      ClientFunc
	maxMessages int
	maxTokens   int
	keepRecent  int
}

// NewCompactor builds a Compactor over the chat history store and a
// per-call chat-client resolver. Zero / out-of-range config fields fall back
// to the package defaults.
func NewCompactor(store history.Store, client ClientFunc, cfg CompactionConfig) *Compactor {
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
// [agentexec.CompactionResult] reports whether the sweep fired and the
// before/after message counts so callers can both chain follow-on
// work (e.g. extraction) and surface an observable boundary event.
//
// No-op (zero result) on a nil receiver (compaction disabled) or an
// empty sessionID.
//
// Important: the summary call goes through chat.Client directly
// (no middleware), so it does NOT enter the chat history middleware
// — otherwise the summarisation request itself would be appended
// to the history and trigger another compaction round.
func (c *Compactor) MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (agentexec.CompactionResult, error) {
	if c == nil || sessionID == "" {
		return agentexec.CompactionResult{}, nil
	}
	msgs, err := c.store.Read(ctx, sessionID)
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: read: %w", err)
	}
	if !c.shouldCompact(msgs) {
		return agentexec.CompactionResult{}, nil
	}
	// The whole history is within keep-recent — nothing OLDER to summarize, and
	// computing a cutoff here would go negative (len-keepRecent < 0 → an
	// out-of-range index in the boundary scan below). This is reachable: the
	// token-footprint trigger ([shouldCompact]) fires on a SHORT conversation
	// bloated by a few large tool results, where len(msgs) < keepRecent. Skip —
	// you can't compact messages you must keep.
	if len(msgs) <= c.keepRecent {
		return agentexec.CompactionResult{}, nil
	}

	// PreCompact hook gate: compaction is now committed (triggers + guards
	// passed), so this fires exactly when a compaction would run — a hook may
	// veto it. nil = always proceed.
	if preCompact != nil && !preCompact(ctx) {
		return agentexec.CompactionResult{}, nil
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
		if msgs[cutoff].Role == chat.RoleUser {
			break
		}
		cutoff++
	}
	if cutoff >= len(msgs) {
		// No clean UserMessage boundary in the trailing segment —
		// skip this compaction cycle rather than corrupt the history.
		return agentexec.CompactionResult{}, nil
	}

	older := msgs[:cutoff]
	recent := msgs[cutoff:]

	summary, err := c.summarize(ctx, older)
	if err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: summarize: %w", err)
	}

	rewritten := make([]chat.Message, 0, 1+len(recent))
	rewritten = append(rewritten, summary)
	rewritten = append(rewritten, recent...)
	// Atomically swap the history for [summary, ...recent] via history.Replace —
	// a transactional backend rolls back on a failed rewrite, so a crash can't
	// leave the conversation cleared-but-not-rewritten (losing `recent` too).
	if err := history.Replace(ctx, c.store, sessionID, rewritten...); err != nil {
		return agentexec.CompactionResult{}, fmt.Errorf("compactor: replace: %w", err)
	}
	return agentexec.CompactionResult{
		Compacted:      true,
		MessagesBefore: before,
		MessagesAfter:  len(rewritten),
	}, nil
}
