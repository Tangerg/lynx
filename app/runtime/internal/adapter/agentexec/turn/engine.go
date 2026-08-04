package turn

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/core/chat"
)

// engineDep is the controller's consumer-side view of Agent execution. The
// concrete agentexec.Engine remains behind this two-method process boundary;
// steering and maintenance are separate dependencies because they have
// different owners and lifecycles.
type engineDep interface {
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
	SubagentProjection(processID string) (agentexec.SubagentProjection, bool)
}

// SteeringSink persists queued steering after the current Run finishes.
type SteeringSink interface {
	AppendUserMessage(ctx context.Context, sessionID string, message chat.Message) error
}

// CompactionResult reports one Run-boundary compaction sweep.
type CompactionResult struct {
	Compacted      bool
	MessagesBefore int
	MessagesAfter  int
}

// RunMaintenance owns the best-effort housekeeping that follows a clean Run.
// The controller supplies immutable Run facts, records returned failures on
// the execution span, and publishes a compaction boundary; the
// implementation owns the workers' ordering and conditional work.
type RunMaintenance interface {
	Maintain(context.Context, RunMaintenanceInput) RunMaintenanceResult
}

// RunMaintenanceInput is the finished Run's maintenance context.
// ModelSelection identifies the model pinned by this Run; an unset selection
// leaves compaction to its configured fallback window. PreCompact is invoked
// only when a compaction is about to commit and may veto it.
type RunMaintenanceInput struct {
	SessionID      string
	Cwd            string
	ModelSelection modelref.Selection
	ToolCalls      int
	PreCompact     func(context.Context) bool
}

// RunMaintenanceResult reports the observable outcome of one maintenance
// sweep. Errors are independent best-effort failures: they never rewrite an
// already-completed user reply.
type RunMaintenanceResult struct {
	Compaction CompactionResult
	Errors     []error
}

// ToolPresenter owns tool-specific activity and result projection. This
// execution adapter owns only lifecycle translation; concrete tool names and schemas remain
// in the tool catalog adapter that implements this interface.
type ToolPresenter interface {
	Activity(toolName string, arguments tool.Arguments) string
	Present(toolName string, arguments tool.Arguments, result tool.Result) (tool.Result, string)
}

// ToolSemantics translates concrete tool names and argument schemas into the
// domain facts used by approval and transcript projection.
type ToolSemantics interface {
	SafetyClass(toolName string) tool.SafetyClass
	ApprovalSubject(toolName string, arguments tool.Arguments) (string, error)
	ShellCommand(toolName, arguments string) string
}
