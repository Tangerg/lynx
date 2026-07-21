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

	// Usage returns subtree-aggregated cost, token, and action totals.
	Usage() (cost float64, tokens int, actions int)

	// ModelCalls returns a defensive copy of subtree LLM history.
	ModelCalls() []ModelCall

	// EmbeddingCalls returns a defensive copy of subtree embedding
	// history.
	EmbeddingCalls() []EmbeddingCall
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

	// Suspend parks durable, JSON-safe continuation state until an external
	// caller responds through runtime.Engine.Resume.
	Suspend(ctx context.Context, suspension interaction.Suspension) (ActionStatus, error)
}

// UsageRecorder is the accounting mutation capability installed privately on
// a ProcessContext. It remains available to isolated parallel branches because
// budget aggregation is concurrency-safe and append-only.
type UsageRecorder interface {
	// RecordUsage attributes a flat (cost, tokens) pair to this
	// process for callers that don't care about per-invocation
	// detail. Equivalent to [UsageRecorder.RecordModelCall] with an
	// ModelCall whose only populated fields are Cost and
	// PromptTokens (PromptTokens stands in for the lumped token
	// count). Prefer the typed Record* methods when per-call audit
	// is required. Invalid or overflowing usage is rejected without
	// mutating the ledger.
	RecordUsage(ctx context.Context, cost float64, tokens int) error

	// RecordModelCall appends an LLM call to this process's
	// invocation history and contributes to subtree budget
	// aggregation. Integration code (chat middleware, per-vendor
	// adapter) calls this once per LLM response. A zero Timestamp is
	// populated at the recording boundary before validation.
	RecordModelCall(ctx context.Context, call ModelCall) error

	// RecordEmbeddingCall appends an embedding call to this
	// process's history. Mirrors RecordModelCall for the
	// embeddings path. A zero Timestamp is populated before validation.
	RecordEmbeddingCall(ctx context.Context, call EmbeddingCall) error
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
