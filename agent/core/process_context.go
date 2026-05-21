package core

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
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

// ProcessContextConfig is the runtime-internal input bundle for
// [NewProcessContext]. The runtime fills it once per tick and calls
// NewProcessContext — keeping the field-injection plumbing inside one
// constructor instead of scattered setter methods on the public surface.
type ProcessContextConfig struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	// ChatClient is the shared LLM client surfaced to action bodies
	// via [ProcessContext.Chat] and [ProcessContext.ChatWithActionTools].
	// nil when the platform was constructed without one — pc.Chat()
	// then returns nil and the action body must handle that case.
	ChatClient *chat.Client

	// Guardrails carries platform-wide chat middlewares (logger /
	// safeguard / quota etc.) that wrap every Chat request action
	// bodies make. nil or empty means "no global middleware".
	Guardrails *Guardrails

	// ActionToolGroups carries the currently-executing action's declared
	// [ToolGroupRequirement]s, so [ProcessContext.ActionTools] can
	// resolve them without the action body having to re-state role
	// names. Mirrors embabel's OperationContext.toolGroups, which reads
	// action.toolGroups for the LLM ops layer.
	ActionToolGroups []ToolGroupRequirement

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
// Every service the action might need lives behind a method here so future
// refactors don't ripple through every action body.
type ProcessContext struct {
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	chatClient       *chat.Client
	guardrails       *Guardrails
	actionToolGroups []ToolGroupRequirement
	publishEvent     EventPublisher
	resolveTools     ToolResolver
	toolCallCancel   ToolCallCanceller

	// lastErr captures the most recent error from a typed-action body so
	// the runtime can extract a ReplanRequest. ProcessContext is built
	// fresh per tick (see runtime.buildProcessContext) and never shared
	// across goroutines, so no synchronisation is needed.
	lastErr error
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
		resolveTools:     config.ResolveTools,
		toolCallCancel:   config.ToolCallCancel,
	}
}

// Tracer returns the framework's OTel tracer so actions can mint custom
// spans without importing otel themselves.
func (pc *ProcessContext) Tracer() trace.Tracer { return agentTracer }

// Publish delivers an event to the runtime's listeners. The `any`-typed
// signature avoids a hard dep on the event package from core.
func (pc *ProcessContext) Publish(event any) {
	if pc.publishEvent == nil {
		return
	}
	pc.publishEvent(event)
}

// ResolveTools turns a list of role names into concrete tools via the
// platform-configured resolver. Returns (nil, nil) when no resolver
// is wired or no roles are supplied; the caller decides whether
// missing tools are fatal.
func (pc *ProcessContext) ResolveTools(ctx context.Context, roles ...string) ([]AgentTool, error) {
	if pc.resolveTools == nil {
		return nil, nil
	}
	return pc.resolveTools(ctx, roles)
}

// Chat returns a fresh [chat.ClientRequest] cloned from the platform's
// shared [chat.Client], or nil when the platform was constructed
// without one — actions that expect LLM access should nil-check (or
// use [ChatWithActionTools] which surfaces a clear error).
//
// Platform-level [Guardrails] (when configured) are pre-installed on
// the returned request — every call / stream the action issues
// passes through the global logger / safeguard / quota middlewares
// before reaching the underlying model.
func (pc *ProcessContext) Chat() *chat.ClientRequest {
	if pc.chatClient == nil {
		return nil
	}
	req := pc.chatClient.Chat()
	if mws := pc.guardrails.MiddlewareValues(); len(mws) > 0 {
		req = req.WithMiddlewares(mws...)
	}
	return req
}

// ChatWithActionTools is the "ask the LLM with my action's tools"
// shortcut: a [chat.ClientRequest] pre-loaded with the action's
// resolved tools and [chat.NewToolMiddleware]. When the action
// declares no ToolGroups, returns the bare client clone.
//
// Platform-level [Guardrails] are layered outside the tool middleware
// so the guardrails see the user-facing request shape (before the
// tool loop expands it).
//
// Errors when no ChatClient is configured or tool resolution fails.
func (pc *ProcessContext) ChatWithActionTools(ctx context.Context) (*chat.ClientRequest, error) {
	if pc.chatClient == nil {
		return nil, errors.New("chat with action tools: no ChatClient configured on the platform")
	}
	req := pc.chatClient.Chat()
	tools, err := pc.ActionTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat with action tools: %w", err)
	}

	guardrails := pc.guardrails.MiddlewareValues()
	if len(tools) == 0 {
		if len(guardrails) > 0 {
			req = req.WithMiddlewares(guardrails...)
		}
		return req, nil
	}
	callMW, streamMW := chat.NewToolMiddleware()
	chain := make([]any, 0, len(guardrails)+2)
	chain = append(chain, guardrails...)
	chain = append(chain, callMW, streamMW)
	return req.WithMiddlewares(chain...).WithTools(tools...), nil
}

// ActionTools resolves the tools declared on the currently-executing
// action's [ActionConfig.ToolGroups]. Returns (nil, nil) when the
// action declared no ToolGroups or no resolver is wired.
func (pc *ProcessContext) ActionTools(ctx context.Context) ([]AgentTool, error) {
	if pc.resolveTools == nil || len(pc.actionToolGroups) == 0 {
		return nil, nil
	}
	roles := make([]string, 0, len(pc.actionToolGroups))
	for _, req := range pc.actionToolGroups {
		roles = append(roles, req.Role)
	}
	return pc.resolveTools(ctx, roles)
}

// ToolCallContext derives a child context the runtime can cancel via
// [Process.TerminateToolCall]. The returned cancel func MUST be
// deferred — it both cancels the ctx and detaches the runtime's
// pointer so a later TerminateToolCall doesn't fire on a stale ctx.
// Without a registered canceller, behaviour falls back to plain
// [context.WithCancel] (TerminateToolCall becomes a no-op).
func (pc *ProcessContext) ToolCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	if pc.toolCallCancel == nil {
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

// AwaitInput delegates to [Process.AwaitInput] — convenience because
// action code already has pc.
func (pc *ProcessContext) AwaitInput(req Awaitable) ActionStatus {
	if pc.Process == nil {
		return ActionFailed
	}
	return pc.Process.AwaitInput(req)
}

// RecordUsage attributes an LLM call's cost / tokens to the running
// process. No-op when no Process is wired.
func (pc *ProcessContext) RecordUsage(cost float64, tokens int) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordUsage(cost, tokens)
}

// RecordLLMInvocation forwards a per-call LLM invocation record to
// the running process. No-op when no Process is wired.
func (pc *ProcessContext) RecordLLMInvocation(inv LLMInvocation) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordLLMInvocation(inv)
}

// RecordEmbeddingInvocation forwards a per-call embedding invocation
// record to the running process. No-op when no Process is wired.
func (pc *ProcessContext) RecordEmbeddingInvocation(inv EmbeddingInvocation) {
	if pc.Process == nil {
		return
	}
	pc.Process.RecordEmbeddingInvocation(inv)
}

// ExecuteSafely runs a.Execute(ctx, pc) under a panic guard,
// recording any recovered panic on the context (inspect via
// [ProcessContext.LastError]). A panic forces [ActionFailed].
func (pc *ProcessContext) ExecuteSafely(ctx context.Context, a Action) (status ActionStatus) {
	if a == nil {
		pc.recordError(errors.New("execute action: action is nil"))
		return ActionFailed
	}
	defer func() {
		if r := recover(); r != nil {
			pc.recordPanic(r)
			status = ActionFailed
		}
	}()
	return a.Execute(ctx, pc)
}

// recordError stashes err for the runtime to detect [ReplanRequest].
func (pc *ProcessContext) recordError(err error) { pc.lastErr = err }

// recordPanic converts a recovered panic value into an error and
// stashes it. Used by [ExecuteSafely].
func (pc *ProcessContext) recordPanic(panicValue any) {
	err, ok := panicValue.(error)
	if !ok {
		err = fmt.Errorf("action panicked: %v", panicValue)
	}
	pc.recordError(err)
}

// LastError returns the last error recorded via recordError (or nil).
func (pc *ProcessContext) LastError() error { return pc.lastErr }

// ResetError clears the per-call error slot. The runtime calls this
// between retries.
func (pc *ProcessContext) ResetError() { pc.lastErr = nil }
