package core

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ProcessView is the read-only process capability used by conditions,
// policies, listeners, middleware, and action bodies. Blackboard returns only
// its reader surface so observers cannot mutate planner state through a
// process reference.
type ProcessView interface {
	ID() string
	ParentID() string
	// SpawnCallID is the parent process's tool-call identity that created this
	// process. It is empty for roots and for children created directly through
	// RunChild rather than through an AgentTool.
	SpawnCallID() string
	Deployment() DeploymentRef
	StartedAt() time.Time
	Status() ProcessStatus
	Goal() GoalDescriptor
	Blackboard() BlackboardReader
	Failure() error
	Suspension() *interaction.Suspension

	// WorldState returns the most recent state observed by the planner.
	WorldState() WorldState

	// Usage returns subtree-aggregated execution resource totals.
	Usage() Usage
}

// ProcessControl is the lifecycle mutation capability installed privately on
// a ProcessContext. Parallel workflow branches intentionally receive none.
type ProcessControl interface {
	TerminateAgent(reason string)
	TerminateAction(reason string)

	// TerminateToolCall cancels any in-flight tool call running through
	// a context derived from [ProcessContext.ToolCallContext]. Action
	// bodies opt in by deriving their tool-invocation contexts from
	// that helper; calls made with a raw ctx receive no cancellation
	// signal. Calling TerminateToolCall when no tool call is active is
	// a no-op.
	TerminateToolCall()

	// Suspend parks JSON-safe continuation state until an external
	// caller responds through runtime.Engine.Resume.
	Suspend(ctx context.Context, suspension interaction.Suspension) (ActionStatus, error)
}

// processViewCtxKey is the unexported context key for embedding a read-only
// ProcessView. Lifecycle and accounting capabilities deliberately never enter
// ambient context; actions receive those only through ProcessContext methods.
type processViewCtxKey struct{}

type processViewContextValue struct {
	process ProcessView
}

// WithProcessView attaches a read-only process view to ctx so nested policy
// helpers can inspect execution state without receiving lifecycle control. A
// nil process masks an inherited view while preserving cancellation and other
// context values.
func WithProcessView(ctx context.Context, process ProcessView) context.Context {
	return context.WithValue(ctx, processViewCtxKey{}, processViewContextValue{process: process})
}

// ProcessViewFrom retrieves the view previously attached via WithProcessView.
// A missing view returns nil so utility code can call it speculatively.
func ProcessViewFrom(ctx context.Context) ProcessView {
	if ctx == nil {
		return nil
	}
	value, _ := ctx.Value(processViewCtxKey{}).(processViewContextValue)
	return value.process
}

// Result pulls the most-recent T from a process's blackboard.
// A top-level function because Go can't have method-level type parameters.
func Result[T any](process ProcessView) (T, bool) {
	var zero T
	if process == nil {
		return zero, false
	}
	return Last[T](process.Blackboard())
}
