package core

import (
	"context"

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

// ToolResolver is the callable the runtime installs so actions can convert
// their declared [ToolGroupRequirement]s into concrete tools without
// importing the runtime. The full requirement flows through (not just the
// role) so the permission check at the resolver dispatch site sees the
// privileges the action opted into — see [ToolGroupRequirement.Permissions].
type ToolResolver func(ctx context.Context, requirements []ToolGroupRequirement) ([]AgentTool, error)

// EventPublisher is the callable the runtime installs for actions to push
// custom events through the multicast listener.
type EventPublisher func(event any)

// ContextEventPublisher is the context-aware event publishing hook. Runtimes
// use it to preserve the current action / run trace when an action calls
// [ProcessContext.Publish] or [ProcessContext.PublishContext].
type ContextEventPublisher func(ctx context.Context, event any)

// ToolCallCancelFunc is the runtime's hook for [ProcessContext.ToolCallContext].
// It hands the runtime a cancel func tied to the in-flight tool call (so
// [Process.TerminateToolCall] can fire it) and returns a release closure
// the caller MUST defer to detach the registration once the tool call
// returns. nil disables tool-call cancellation: ToolCallContext returns
// the parent ctx unchanged.
type ToolCallCancelFunc func(cancel context.CancelFunc) (release func())

// ProcessScope bundles the per-process fields — constant across
// every tick of the same process. The runtime carries one of these
// per AgentProcess and threads it into the ProcessContext built at
// each tick.
type ProcessScope struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider
}

// PlatformHooks bundles the platform-wired callbacks — installed
// once at Platform construction time and reused across every tick
// of every process. nil-safe at the ProcessContext methods that
// consume them (e.g. Publish becomes a no-op when nil).
type PlatformHooks struct {
	// ChatClient is the shared LLM client surfaced to action bodies
	// via [ProcessContext.Chat] and [ProcessContext.ChatWithActionTools].
	// nil when the platform was constructed without one — pc.Chat()
	// then returns nil and the action body must handle that case.
	ChatClient ChatClient

	// Guardrails carries platform-wide chat middlewares (logger /
	// safeguard / quota etc.) that wrap every Chat request action
	// bodies make. nil or empty means "no global middleware".
	Guardrails *Guardrails

	// Publish is invoked by [ProcessContext.Publish]; nil makes Publish
	// a no-op. The runtime supplies a closure that fans the event out
	// to the platform's multicast listener.
	Publish EventPublisher

	// PublishContext is the context-aware companion to Publish. When set,
	// [ProcessContext.Publish] uses the current action ctx captured by
	// [ProcessContext.ExecuteSafely], and [ProcessContext.PublishContext]
	// forwards the caller-supplied ctx.
	PublishContext ContextEventPublisher

	// ResolveTools is invoked by [ProcessContext.ResolveTools]; nil
	// makes ResolveTools return (nil, nil). The runtime supplies a
	// closure backed by the platform's [ToolGroupResolver].
	ResolveTools ToolResolver

	// ToolCallCancel registers a cancel func and returns a release
	// closure — single function rather than a register/clear pair so
	// callers can't mismatch them.
	ToolCallCancel ToolCallCancelFunc
}

// ProcessContextConfig is the runtime-internal input bundle for
// [NewProcessContext]. The runtime fills it once per tick — keeping
// the field-injection plumbing inside one constructor instead of
// scattered setter methods on the public surface.
//
// Three concerns physically split via embedded sub-structs:
//
//   - [ProcessScope]   — constant across a process's lifetime
//   - [PlatformHooks]  — constant across the Platform's lifetime
//   - ActionToolGroups — refreshed every tick from the
//     currently-executing action's declared requirements
type ProcessContextConfig struct {
	ProcessScope
	PlatformHooks

	// ActionToolGroups carries the currently-executing action's declared
	// [ToolGroupRequirement]s, so [ProcessContext.ActionTools] can
	// resolve them without the action body having to re-state role names.
	ActionToolGroups []ToolGroupRequirement
}

// ProcessContext is the only thing handed to an [Action.Execute] call.
// Every service the action might need lives behind a method here so
// future refactors don't ripple through every action body.
//
// Field grouping mirrors [ProcessContextConfig]: public state up top,
// platform-wired hooks in the middle (held privately so callers go
// through the typed methods), per-action state + per-tick scratch at
// the bottom.
type ProcessContext struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	// Platform-wired hooks. Private so action bodies go through
	// the typed methods (Chat / Publish / ResolveTools / ...) instead
	// of touching the underlying client / closure directly.
	chatClient     ChatClient
	guardrails     *Guardrails
	publishEvent   EventPublisher
	publishContext ContextEventPublisher
	resolveTools   ToolResolver
	toolCallCancel ToolCallCancelFunc

	actionToolGroups []ToolGroupRequirement

	// inputAwaited flips when the action calls [AwaitInput]; the
	// typed-action wrapper reads it to return ActionWaiting. Per-tick
	// (fresh ProcessContext each invocation), so no reset needed.
	inputAwaited bool

	// lastErr captures the most recent error from a typed-action body so
	// the runtime can extract a ReplanRequest. ProcessContext is built
	// fresh per tick (see runtime.buildProcessContext) and owned by a single
	// goroutine, so these two scratch fields need no synchronization. A
	// concurrent fan-out (the workflow ScatterGather / Consensus generators)
	// preserves that invariant by handing each branch its own copy via
	// [ProcessContext.ForParallelBranch] rather than sharing one.
	lastErr error

	// eventContext is the current action ctx captured during ExecuteSafely.
	// Publish uses it so action-emitted events inherit the same trace as the
	// action body without changing existing action code.
	eventContext context.Context
}

// NewProcessContext assembles a ProcessContext from config. Used by the
// runtime once per tick; users don't construct ProcessContexts themselves.
func NewProcessContext(config ProcessContextConfig) *ProcessContext {
	return &ProcessContext{
		Process:          config.Process,
		Blackboard:       config.Blackboard,
		Options:          config.Options,
		OutputChannel:    config.OutputChannel,
		Services:         config.Services,
		chatClient:       config.ChatClient,
		guardrails:       config.Guardrails,
		actionToolGroups: config.ActionToolGroups,
		publishEvent:     config.Publish,
		publishContext:   config.PublishContext,
		resolveTools:     config.ResolveTools,
		toolCallCancel:   config.ToolCallCancel,
	}
}

// ForParallelBranch returns a sibling-safe copy of pc for a goroutine running
// concurrently with other branches of the SAME action — the workflow fan-out
// builders (ScatterGather / Consensus / Parallel) hand one to each generator.
//
// It shares the process-level state (Process / Blackboard / Services) and the
// platform hooks — all safe for concurrent use — but gets its OWN per-invocation
// scratch (inputAwaited / lastErr), the two plain unsynchronized fields a single
// goroutine is assumed to own. Without this, N branches sharing one
// ProcessContext would race those fields (e.g. two generators calling
// AwaitInput). The runtime's per-tick path uses [NewProcessContext] for the same
// "one ProcessContext per goroutine" guarantee.
func (pc *ProcessContext) ForParallelBranch() *ProcessContext {
	branch := *pc
	branch.inputAwaited = false
	branch.lastErr = nil
	return &branch
}

// Tracer returns the framework's OTel tracer so actions can mint custom
// spans without importing otel themselves.
func (pc *ProcessContext) Tracer() trace.Tracer { return agentTracer }
