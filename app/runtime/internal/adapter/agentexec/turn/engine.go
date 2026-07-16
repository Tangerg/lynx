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
type Compactor interface {
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (CompactionResult, error)
}

// ExtractionResult reports facts appended to project memory.
type ExtractionResult struct {
	Extracted bool
	Facts     string
}

// Extractor mines recent conversation facts after a successful compaction.
type Extractor interface {
	MaybeExtract(ctx context.Context, sessionID, cwd string) (ExtractionResult, error)
}
