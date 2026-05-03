package core

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// agentTracer is the framework-wide OTel tracer. We deliberately don't
// expose a TracerProvider abstraction — callers configure OTel globally
// (see doc/OBSERVABILITY.md) and the agent layer takes whatever's installed.
var agentTracer = otel.Tracer("lynx/agent")

// AgentTracer exposes the tracer for adapters that want to mint child spans
// outside Action.Execute (e.g. event listeners building debug spans).
func AgentTracer() trace.Tracer { return agentTracer }

// ToolResolver is the callable the runtime installs so actions can convert a
// list of role names into concrete tools without importing the runtime.
type ToolResolver func(ctx context.Context, roles []string) ([]AgentTool, error)

// EventPublisher is the callable the runtime installs for actions to push
// custom events through the multicast listener.
type EventPublisher func(event any)

// ProcessContext is the only thing handed to an Action.Execute call. Every
// service the action might need lives behind a method here so future
// refactors (swap chat client, change RAG implementation) don't ripple
// through every action body.
type ProcessContext struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	publishEvent EventPublisher
	resolveTools ToolResolver

	// errSlot captures the most recent error from a typed-action body so the
	// runtime can extract a ReplanRequest. Atomic store so concurrent panic
	// recovery and normal-return paths don't race.
	errSlot atomic.Pointer[error]
	lastErr error
}

// Tracer returns the framework's OTel tracer so actions can mint custom
// spans without importing otel themselves.
func (pc *ProcessContext) Tracer() trace.Tracer { return agentTracer }

// Publish delivers an event to the runtime's listeners. The `any`-typed
// signature lets us avoid a hard dep on the event package from core.
func (pc *ProcessContext) Publish(event any) {
	if pc == nil || pc.publishEvent == nil {
		return
	}
	pc.publishEvent(event)
}

// SetPublishFunc is called by the runtime when wiring up the ProcessContext.
// Exposed instead of made-private because the runtime lives in a sibling
// package and Go visibility wouldn't let it set the field otherwise.
func (pc *ProcessContext) SetPublishFunc(fn EventPublisher) {
	pc.publishEvent = fn
}

// SetResolveToolsFunc installs the lazy tool resolver. Same visibility
// rationale as SetPublishFunc.
func (pc *ProcessContext) SetResolveToolsFunc(fn ToolResolver) {
	pc.resolveTools = fn
}

// ResolveTools turns a list of role names into concrete tools via the
// platform-configured resolver. Roles that don't resolve are skipped
// silently — the action is responsible for deciding whether the missing
// tools are fatal.
func (pc *ProcessContext) ResolveTools(ctx context.Context, roles ...string) ([]AgentTool, error) {
	if pc == nil || pc.resolveTools == nil {
		return nil, nil
	}
	return pc.resolveTools(ctx, roles)
}

// AwaitInput delegates to the underlying Process.AwaitInput. Convenience
// because action code already has pc, not the bare process.
func (pc *ProcessContext) AwaitInput(req Awaitable) ActionStatus {
	if pc == nil || pc.Process == nil {
		return ActionFailed
	}
	return pc.Process.AwaitInput(req)
}

// recordError lets the typed-action wrapper stash the underlying error so
// the runtime can detect ReplanRequest later. Internal — exported only
// because the typed-action constructor is in the same package.
func (pc *ProcessContext) recordError(err error) {
	if pc == nil {
		return
	}
	pc.lastErr = err
	pc.errSlot.Store(&err)
}

// RecordPanic stashes a recovered panic value as the underlying error. The
// runtime's executeAction panic-recovery path uses this to surface the panic
// uniformly with normal errors.
func (pc *ProcessContext) RecordPanic(panicValue any) {
	if pc == nil {
		return
	}

	err, ok := panicValue.(error)
	if !ok {
		err = fmt.Errorf("action panicked: %v", panicValue)
	}
	pc.recordError(err)
}

// LastError returns the last error recorded via recordError (or nil).
func (pc *ProcessContext) LastError() error {
	if pc == nil {
		return nil
	}
	if stored := pc.errSlot.Load(); stored != nil {
		return *stored
	}
	return pc.lastErr
}

// ResetError clears the per-call error slot. The runtime calls this between
// retries so a stale error from attempt N doesn't leak into the diagnosis
// of attempt N+1's status.
func (pc *ProcessContext) ResetError() {
	if pc == nil {
		return
	}
	pc.lastErr = nil
	pc.errSlot.Store(nil)
}
