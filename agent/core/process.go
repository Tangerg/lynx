package core

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ErrUsageUnavailable reports that a ProcessContext has no accounting owner.
var ErrUsageUnavailable = errors.New("agent: process usage recorder is unavailable")

// ProcessView is the read-only process capability used by conditions,
// policies, listeners, middleware, and action bodies. Blackboard returns only
// its reader surface so observers cannot mutate planner state through a
// process reference.
type ProcessView interface {
	ID() string
	ParentID() string
	Deployment() DeploymentRef
	StartedAt() time.Time
	Status() ProcessStatus
	Goal() *Goal
	Blackboard() BlackboardReader
	Failure() error
	Suspension() *interaction.Suspension

	// WorldState returns the most recent state observed by the planner.
	WorldState() WorldState

	// Usage returns subtree-aggregated execution resource totals.
	Usage() ProcessUsage
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

// UsageRecorder is the execution-resource mutation capability installed
// privately on a ProcessContext. It remains available to isolated parallel
// branches because aggregation is concurrency-safe and append-only.
type UsageRecorder interface {
	// RecordUsage attributes one aggregate delta to this process. Invalid or
	// overflowing usage is rejected without mutating runtime state.
	RecordUsage(ctx context.Context, usage Usage) error
}

// processViewCtxKey is the unexported context key for embedding a read-only
// ProcessView. Lifecycle and accounting capabilities deliberately never enter
// ambient context; actions receive those only through ProcessContext methods.
type processViewCtxKey struct{}

// WithProcessView attaches a read-only process view to ctx so nested policy
// helpers can inspect execution state without receiving lifecycle control.
func WithProcessView(ctx context.Context, process ProcessView) context.Context {
	return context.WithValue(ctx, processViewCtxKey{}, process)
}

// ProcessViewFrom retrieves the view previously attached via WithProcessView.
// A missing view returns nil so utility code can call it speculatively.
func ProcessViewFrom(ctx context.Context) ProcessView {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(processViewCtxKey{}).(ProcessView)
	return p
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
