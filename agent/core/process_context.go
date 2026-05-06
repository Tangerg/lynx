package core

import (
	"context"
	"fmt"

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

// ToolCallCanceller is the runtime's hook for [ProcessContext.ToolCallContext].
// It hands the runtime a cancel func tied to the in-flight tool call (so
// [Process.TerminateToolCall] can fire it) and returns a release closure
// the caller MUST defer to detach the registration once the tool call
// returns. nil disables tool-call cancellation: ToolCallContext returns
// the parent ctx unchanged.
type ToolCallCanceller func(cancel context.CancelFunc) (release func())

// ProcessContextDeps bundles every dependency [NewProcessContext] needs.
// The runtime fills it once per tick and calls NewProcessContext — that
// keeps the field-injection plumbing inside one constructor instead of
// scattered setter methods on the public surface.
type ProcessContextDeps struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	// Publish is invoked by [ProcessContext.Publish]; nil makes Publish
	// a no-op. The runtime supplies a closure that fans the event out
	// to the platform's multicast listener.
	Publish EventPublisher

	// ResolveTools is invoked by [ProcessContext.ResolveTools]; nil
	// makes ResolveTools return (nil, nil). The runtime supplies a
	// closure backed by the platform's [ToolGroupResolver].
	ResolveTools ToolResolver

	// ToolCallCancel registers a cancel func and returns a release
	// closure — single function rather than a register/clear pair so
	// callers can't mismatch them.
	ToolCallCancel ToolCallCanceller
}

// ProcessContext is the only thing handed to an [Action.Execute] call.
// Every dependency the action body might need is reachable through a
// read-only accessor (Process / Blackboard / Options / OutputChannel /
// Services) so future refactors don't ripple through every action.
//
// Fields are intentionally private: ProcessContext is built once per
// tick by the runtime, immutable for the duration of the action call,
// and discarded — user code reads, never writes.
type ProcessContext struct {
	process       Process
	blackboard    Blackboard
	options       *ProcessOptions
	outputChannel OutputChannel
	services      *ServiceProvider

	publishEvent   EventPublisher
	resolveTools   ToolResolver
	toolCallCancel ToolCallCanceller

	// lastErr captures the most recent error from a typed-action body so
	// the runtime can extract a ReplanRequest. ProcessContext is built
	// fresh per tick (see runtime.buildProcessContext) and never shared
	// across goroutines, so no synchronisation is needed.
	lastErr error
}

// NewProcessContext assembles a ProcessContext from deps. Used by the
// runtime once per tick; users don't construct ProcessContexts themselves.
func NewProcessContext(deps ProcessContextDeps) *ProcessContext {
	return &ProcessContext{
		process:        deps.Process,
		blackboard:     deps.Blackboard,
		options:        deps.Options,
		outputChannel:  deps.OutputChannel,
		services:       deps.Services,
		publishEvent:   deps.Publish,
		resolveTools:   deps.ResolveTools,
		toolCallCancel: deps.ToolCallCancel,
	}
}

// --- Read-only accessors -------------------------------------------------

// Process returns the running process this context belongs to.
func (pc *ProcessContext) Process() Process { return pc.process }

// Blackboard returns the runtime blackboard. Action code reads from /
// writes to it via the [Blackboard] interface methods.
func (pc *ProcessContext) Blackboard() Blackboard { return pc.blackboard }

// Options returns the per-process configuration the runtime is using.
func (pc *ProcessContext) Options() *ProcessOptions { return pc.options }

// OutputChannel returns the action-level "say something to the user"
// sink. Defaults to [DevNullOutputChannel] when none is configured.
func (pc *ProcessContext) OutputChannel() OutputChannel { return pc.outputChannel }

// Services returns the open-ended service registry — see [ServiceOf]
// for typed lookups.
func (pc *ProcessContext) Services() *ServiceProvider { return pc.services }

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

// ToolCallContext derives a child context the runtime can cancel via
// [Process.TerminateToolCall]. Action code passes the returned ctx to
// chat clients / tool invocations; the returned cancel func MUST be
// deferred by the caller — it both cancels the ctx (releasing
// resources) and detaches the runtime's pointer to it so a later
// TerminateToolCall doesn't fire on a stale ctx.
//
// When pc has no registered canceller (e.g. tests building a bare
// ProcessContext), behaviour falls back to plain [context.WithCancel] —
// TerminateToolCall becomes a no-op.
func (pc *ProcessContext) ToolCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	if pc == nil || pc.toolCallCancel == nil {
		return ctx, cancel
	}

	release := pc.toolCallCancel(cancel)
	return ctx, func() {
		cancel()
		if release != nil {
			release()
		}
	}
}

// AwaitInput delegates to the underlying Process.AwaitInput. Convenience
// because action code already has pc, not the bare process.
func (pc *ProcessContext) AwaitInput(req Awaitable) ActionStatus {
	if pc == nil || pc.process == nil {
		return ActionFailed
	}
	return pc.process.AwaitInput(req)
}

// RecordUsage attributes an LLM call's cost (USD) and token count to the
// running process. Convenience wrapper around [Process.RecordUsage] so
// action code that's already holding pc doesn't need to reach for the
// bare process. No-op when pc or its process is nil.
func (pc *ProcessContext) RecordUsage(cost float64, tokens int) {
	if pc == nil || pc.process == nil {
		return
	}
	pc.process.RecordUsage(cost, tokens)
}

// ExecuteSafely runs a.Execute(ctx, pc) under a panic guard, recording
// any recovered panic on the context so callers can inspect it via
// [ProcessContext.LastError]. A panic forces the returned status to
// [ActionFailed].
//
// The runtime calls this instead of action.Execute directly so framework
// code never trusts user action bodies to be panic-clean.
func (pc *ProcessContext) ExecuteSafely(ctx context.Context, a Action) (status ActionStatus) {
	defer func() {
		if r := recover(); r != nil {
			pc.recordPanic(r)
			status = ActionFailed
		}
	}()
	return a.Execute(ctx, pc)
}

// recordError lets the typed-action wrapper stash the underlying error
// so the runtime can detect ReplanRequest later.
func (pc *ProcessContext) recordError(err error) {
	if pc == nil {
		return
	}
	pc.lastErr = err
}

// recordPanic converts a recovered panic value into an error and stashes
// it. Used by [ExecuteSafely].
func (pc *ProcessContext) recordPanic(panicValue any) {
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
	return pc.lastErr
}

// ResetError clears the per-call error slot. The runtime calls this
// between retries so a stale error from attempt N doesn't leak into
// the diagnosis of attempt N+1's status.
func (pc *ProcessContext) ResetError() {
	if pc == nil {
		return
	}
	pc.lastErr = nil
}
