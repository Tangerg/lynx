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
	// signals. Calling them outside a tick boundary is
	// allowed; the runtime checks for pending signals at the start of each
	// tick.
	TerminateAgent(reason string)
	TerminateAction(reason string)

	// TerminateToolCall cancels any in-flight tool call running through
	// a context derived from [ProcessContext.ToolCallContext]. Action
	// bodies opt in by deriving their tool-invocation contexts from
	// that helper; calls made with a raw ctx receive no cancellation
	// signal. Calling TerminateToolCall when no tool call is active is
	// a no-op.
	TerminateToolCall()

	// AwaitInput suspends the action until external input arrives. The
	// runtime stores the Awaitable on the blackboard, transitions to
	// StatusWaiting, and returns ActionWaiting.
	AwaitInput(ctx context.Context, req Awaitable) ActionStatus

	// RecordUsage attributes a flat (cost, tokens) pair to this
	// process for callers that don't care about per-invocation
	// detail. Equivalent to [Process.RecordLLMInvocation] with an
	// LLMInvocation whose only populated fields are Cost and
	// PromptTokens (PromptTokens stands in for the lumped token
	// count). Prefer the typed Record* methods when per-call audit
	// is required.
	RecordUsage(ctx context.Context, cost float64, tokens int)

	// RecordLLMInvocation appends an LLM call to this process's
	// invocation history and contributes to subtree budget
	// aggregation. Integration code (chat middleware, per-vendor
	// adapter) calls this once per LLM response.
	RecordLLMInvocation(ctx context.Context, invocation LLMInvocation)

	// RecordEmbeddingInvocation appends an embedding call to this
	// process's history. Mirrors RecordLLMInvocation for the
	// embeddings path.
	RecordEmbeddingInvocation(ctx context.Context, invocation EmbeddingInvocation)

	// Usage returns the subtree-aggregated cost / token / action totals.
	// Cost and tokens come from RecordUsage / RecordLLMInvocation /
	// RecordEmbeddingInvocation calls; the action count is the
	// recursive sum of every History across this process and its
	// child processes. [BudgetPolicy] reads this directly so a
	// parent's budget governs its entire delegation tree.
	Usage() (cost float64, tokens int, actions int)

	// LLMInvocations returns the subtree-aggregated LLM invocation
	// history in chronological order across this process and every
	// descendant. The returned slice is a fresh copy — callers may
	// retain or sort it without affecting future Record* calls.
	LLMInvocations() []LLMInvocation

	// EmbeddingInvocations returns the subtree-aggregated embedding
	// invocation history. Same ordering and copy semantics as
	// LLMInvocations.
	EmbeddingInvocations() []EmbeddingInvocation
}

// processCtxKey is the unexported context key for embedding a Process. Using
// an empty struct ensures collisions are impossible with other packages.
type processCtxKey struct{}

// WithProcess attaches the process to ctx so deeply-nested helpers can find
// it without an extra parameter.
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

// ResultOfType pulls the most-recent T from a process's blackboard.
// A top-level function because Go can't have method-level type parameters.
func ResultOfType[T any](p Process) (T, bool) {
	var zero T
	if p == nil {
		return zero, false
	}
	return Last[T](p.Blackboard())
}
