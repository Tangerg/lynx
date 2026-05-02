package core

import (
	"context"
	"time"
)

// Process is the read surface that actions, conditions, and event listeners
// see — it is intentionally narrower than the runtime's full AgentProcess so
// concrete state-mutation methods (TerminateAgent, suspendForAwaitable,
// etc.) stay on the runtime side. The runtime's *AgentProcess implements
// this.
type Process interface {
	ID() string
	ParentID() string
	StartedAt() time.Time
	Status() AgentProcessStatus
	Goal() *Goal
	Blackboard() Blackboard
	Options() *ProcessOptions
	Failure() error

	// LastWorldState surfaces the snapshot the planner saw on its most
	// recent orient phase. Listeners use it to display "what does the agent
	// currently believe?" without re-running the determiner.
	LastWorldState() WorldState

	// TerminateAgent and TerminateAction are the structured-termination
	// signals from embabel 0.4. Calling them outside a tick boundary is
	// allowed; the runtime checks for pending signals at the start of each
	// tick.
	TerminateAgent(reason string)
	TerminateAction(reason string)

	// AwaitInput suspends the action until external input arrives. The
	// runtime stores the Awaitable on the blackboard, transitions to
	// StatusWaiting, and returns ActionWaiting.
	AwaitInput(req Awaitable) ActionStatus
}

// processCtxKey is the unexported context key for embedding a Process. Using
// an empty struct ensures collisions are impossible with other packages.
type processCtxKey struct{}

// WithProcess attaches the process to ctx so deeply-nested helpers can find
// it without an extra parameter — replaces embabel's ThreadLocal access.
func WithProcess(ctx context.Context, p Process) context.Context {
	return context.WithValue(ctx, processCtxKey{}, p)
}

// ProcessFrom retrieves the process previously attached via WithProcess. A
// missing process returns nil rather than panicking so utility code can call
// it speculatively.
func ProcessFrom(ctx context.Context) Process {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(processCtxKey{}).(Process)
	return p
}

// ResultOfType pulls the most-recent T from a process's blackboard. Mirrors
// embabel's AgentProcess.resultOfType<T>() but as a top-level function (Go
// can't have method-level type parameters).
func ResultOfType[T any](p Process) (T, bool) {
	var zero T
	if p == nil {
		return zero, false
	}
	return Last[T](p.Blackboard())
}
