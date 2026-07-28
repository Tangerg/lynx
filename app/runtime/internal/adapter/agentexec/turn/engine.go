package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
)

// engineDep is the dispatcher's consumer-side view of Agent execution. The
// concrete agentexec.Engine remains behind this two-method process boundary;
// steering and maintenance are separate dependencies because they have
// different owners and lifecycles.
type engineDep interface {
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
	ProcessResult(processID string) (any, bool)
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

// BoundaryMaintenance owns the best-effort housekeeping that follows a clean
// turn. The dispatcher supplies immutable turn facts, records the returned
// failures on the turn span, and publishes a compaction boundary; the
// implementation owns the workers' ordering and conditional work.
type BoundaryMaintenance interface {
	Maintain(context.Context, BoundaryMaintenanceInput) BoundaryMaintenanceResult
}

// BoundaryMaintenanceInput is the finished turn's maintenance context.
// ModelSelection identifies the model pinned by this turn; an unset selection
// leaves compaction to its configured fallback window. PreCompact is invoked
// only when a compaction is about to commit and may veto it.
type BoundaryMaintenanceInput struct {
	SessionID      string
	Cwd            string
	ModelSelection modelref.Selection
	ToolCalls      int
	PreCompact     func(context.Context) bool
}

// BoundaryMaintenanceResult reports the observable outcome of one maintenance
// sweep. Errors are independent best-effort failures: they never rewrite an
// already-completed user reply.
type BoundaryMaintenanceResult struct {
	Compaction CompactionResult
	Errors     []error
}
