package agentexec

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// CompactionResult reports one completed Run-boundary compaction sweep.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// RunMaintenance owns best-effort housekeeping after a clean Interaction but
// before its terminal fact closes the live Segment stream.
type RunMaintenance interface {
	Maintain(ctx context.Context, input RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished root Interaction's maintenance context.
type RunMaintenanceInput struct {
	SessionID      string
	CWD            string
	ModelSelection modelref.Selection
	ToolCalls      int
	PreCompact     func(context.Context) bool
}

// RunMaintenanceResult reports independent best-effort failures without
// rewriting the already-produced assistant response.
type RunMaintenanceResult struct {
	Compaction CompactionResult
	Errors     []error
}

// InteractionLifecycleHooks owns the Runtime lifecycle events that are not
// part of Tool authorization or prompt composition.
type InteractionLifecycleHooks interface {
	BeforeCompaction(ctx context.Context, sessionID, cwd string) bool
	NotifyWaiting(ctx context.Context, sessionID, cwd string)
	NotifyStopped(ctx context.Context, sessionID, cwd, reason string)
}

func (i *interactionSession) maintainCompletedRoot() {
	if i.maintenance == nil || i.start.SessionID == "" {
		return
	}
	toolCalls := i.accounting.toolCallCount()
	preCompact := func(ctx context.Context) bool {
		return i.lifecycleHooks == nil || i.lifecycleHooks.BeforeCompaction(
			ctx, i.start.SessionID, i.start.CWD,
		)
	}
	result := i.maintenance.Maintain(i.lifetime.context, RunMaintenanceInput{
		SessionID:      i.start.SessionID,
		CWD:            i.start.CWD,
		ModelSelection: i.start.ModelSelection,
		ToolCalls:      toolCalls,
		PreCompact:     preCompact,
	})
	for _, err := range result.Errors {
		if err != nil {
			trace.SpanFromContext(i.lifetime.context).RecordError(
				fmt.Errorf("agentexec: Run maintenance: %w", err),
			)
		}
	}
	if result.Compaction.Compacted {
		i.lifetime.send(runs.ExecutorEvent{
			Member: runs.ExecutorMember{MemberID: i.processRootID().String()},
			Payload: runs.CompactionBoundary{
				MessagesBefore: result.Compaction.MessagesBefore,
				MessagesAfter:  result.Compaction.MessagesAfter,
			},
		})
	}
}
