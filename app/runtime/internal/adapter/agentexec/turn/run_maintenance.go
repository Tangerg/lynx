package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// CompactionResult reports one Run-boundary compaction sweep.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// Maintenance owns the best-effort housekeeping that follows a clean Run.
type Maintenance interface {
	Maintain(ctx context.Context, input MaintenanceInput) MaintenanceResult
}

// MaintenanceInput is the finished Run's maintenance context.
type MaintenanceInput struct {
	SessionID      string
	CWD            string
	ModelSelection modelref.Selection
	ToolCalls      int
	PreCompact     func(context.Context) bool
}

// MaintenanceResult reports independent best-effort failures without
// rewriting an already-completed user reply.
type MaintenanceResult struct {
	Compaction CompactionResult
	Errors     []error
}
