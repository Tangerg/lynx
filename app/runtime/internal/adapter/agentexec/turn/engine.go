package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// engineDep is the dispatcher's consumer-side view of Agent execution. The
// concrete agentexec.Engine remains behind this two-method process boundary;
// steering and maintenance are separate dependencies because they have
// different owners and lifecycles.
type engineDep interface {
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
}

// SteeringSink persists queued steering after the current turn finishes.
type SteeringSink interface {
	InjectUser(ctx context.Context, sessionID, text string) error
}

// CompactionResult reports one turn-boundary compaction sweep.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// Compactor folds over-long conversation history at a turn boundary.
// contextWindow is the running model's context window in tokens (0 = unknown),
// so the token-footprint trigger tracks the model this run actually pinned rather
// than a process-wide default.
type Compactor interface {
	MaybeCompact(ctx context.Context, sessionID string, contextWindow int, preCompact func(context.Context) bool) (CompactionResult, error)
}

// Extractor mines recent conversation facts after a successful compaction.
type Extractor interface {
	MaybeExtract(ctx context.Context, sessionID, cwd string) error
}

// SkillMiner distills a complex turn's trajectory into a proposed skill draft.
// It runs at the turn boundary independent of compaction — a complex turn is
// worth capturing whether or not history needed folding — and owns its own
// complexity threshold and cadence, so it decides whether to mine from the
// reported signal. toolCalls is the just-finished turn's completed tool-call
// count, the complexity signal.
type SkillMiner interface {
	MaybeMine(ctx context.Context, sessionID, cwd string, toolCalls int) error
}
