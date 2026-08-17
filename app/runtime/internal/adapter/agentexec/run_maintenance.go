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

func (session *interactionSession) maintainCompletedRoot() {
	if session.maintenance == nil || session.start.SessionID == "" {
		return
	}
	toolCalls := session.accounting.toolCallCount()
	preCompact := func(ctx context.Context) bool {
		return session.lifecycleHooks == nil || session.lifecycleHooks.BeforeCompaction(
			ctx, session.start.SessionID, session.start.CWD,
		)
	}
	result := session.maintenance.Maintain(session.lifetime.context, RunMaintenanceInput{
		SessionID:      session.start.SessionID,
		CWD:            session.start.CWD,
		ModelSelection: session.start.ModelSelection,
		ToolCalls:      toolCalls,
		PreCompact:     preCompact,
	})
	for _, err := range result.Errors {
		if err != nil {
			trace.SpanFromContext(session.lifetime.context).RecordError(
				fmt.Errorf("agentexec: Run maintenance: %w", err),
			)
		}
	}
	if result.Compaction.Compacted {
		session.lifetime.send(runs.ExecutorEvent{
			Member: runs.ExecutorMember{MemberID: session.processRootID().String()},
			Payload: runs.CompactionBoundary{
				MessagesBefore: result.Compaction.MessagesBefore,
				MessagesAfter:  result.Compaction.MessagesAfter,
			},
		})
	}
}
