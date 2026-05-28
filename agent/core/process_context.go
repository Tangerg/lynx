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

// ToolCallCancelFunc is the runtime's hook for [ProcessContext.ToolCallContext].
// It hands the runtime a cancel func tied to the in-flight tool call (so
// [Process.TerminateToolCall] can fire it) and returns a release closure
// the caller MUST defer to detach the registration once the tool call
// returns. nil disables tool-call cancellation: ToolCallContext returns
// the parent ctx unchanged.
type ToolCallCancelFunc func(cancel context.CancelFunc) (release func())

// ProcessContextConfig is the runtime-internal input bundle for
// [NewProcessContext]. The runtime fills it once per tick — keeping
// the field-injection plumbing inside one constructor instead of
// scattered setter methods on the public surface.
//
// Fields divide into three concerns:
//
//   - Per-process state (Process / Blackboard / Options /
//     OutputChannel / Services) — flow through unchanged across
//     ticks of the same process.
//   - Platform-wired hooks (ChatClient / Guardrails / Publish /
//     ResolveTools / ToolCallCancel) — installed by the platform
//     at construction; same value for every tick.
//   - Per-action state (ActionToolGroups) — refreshed at each
//     tick from the currently-executing action.
//
// The struct stays flat for ease of construction at the single
// caller (runtime/execute_action.go:buildProcessContext).
type ProcessContextConfig struct {
	// --- Per-process state (constant across a process's lifetime). ---

	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	// --- Platform-wired hooks (installed once at platform construction). ---

	// ChatClient is the shared LLM client surfaced to action bodies
	// via [ProcessContext.Chat] and [ProcessContext.ChatWithActionTools].
	// nil when the platform was constructed without one — pc.Chat()
	// then returns nil and the action body must handle that case.
	ChatClient *chat.Client

	// Guardrails carries platform-wide chat middlewares (logger /
	// safeguard / quota etc.) that wrap every Chat request action
	// bodies make. nil or empty means "no global middleware".
	Guardrails *Guardrails

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
	ToolCallCancel ToolCallCancelFunc

	// --- Per-action state (refreshed every tick). ---

	// ActionToolGroups carries the currently-executing action's declared
	// [ToolGroupRequirement]s, so [ProcessContext.ActionTools] can
	// resolve them without the action body having to re-state role
	// names. Mirrors embabel's ConditionEnv.toolGroups, which reads
	// action.toolGroups for the LLM ops layer.
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
	// --- Public per-process state. ---
	Process       Process
	Blackboard    Blackboard
	Options       *ProcessOptions
	OutputChannel OutputChannel
	Services      *ServiceProvider

	// --- Platform-wired hooks. Private so action bodies go through
	// the typed methods (Chat / Publish / ResolveTools / ...) instead
	// of touching the underlying client / closure directly.
	chatClient     *chat.Client
	guardrails     *Guardrails
	publishEvent   EventPublisher
	resolveTools   ToolResolver
	toolCallCancel ToolCallCancelFunc

	// --- Per-action state + per-tick scratch. ---
	actionToolGroups []ToolGroupRequirement

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
//
// When [ProcessOptions.Session] is set (typically via
// [Platform.RunInSession]) the session id is stamped onto the
// request params under the chat-memory conversation key so the
// memory middleware auto-loads the conversation history.
func (pc *ProcessContext) Chat() *chat.ClientRequest {
	if pc.chatClient == nil {
		return nil
	}
	return pc.buildChatRequest(nil)
}

// ChatWithActionTools is the "ask the LLM with my action's tools"
// shortcut: a [chat.ClientRequest] pre-loaded with the action's
// resolved tools and [chat.NewToolMiddleware]. When the action
// declares no ToolGroups, returns the bare client clone (same shape
// as [Chat]).
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
	tools, err := pc.ActionTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat with action tools: %w", err)
	}
	return pc.buildChatRequest(tools), nil
}

// buildChatRequest composes the per-action chat request: guardrails
// outermost, optional tool middleware innermost. Callers pre-resolve
// tools (nil means "no tool middleware"); the rest of the wiring
// (chatClient existence, session params, middleware order) lives in
// one place so [Chat] and [ChatWithActionTools] stay aligned.
func (pc *ProcessContext) buildChatRequest(tools []AgentTool) *chat.ClientRequest {
	req := pc.chatClient.Chat()

	mws := pc.guardrails.MiddlewareValues()
	if len(tools) > 0 {
		callMW, streamMW := chat.NewToolMiddleware()
		mws = append(mws, callMW, streamMW)
	}
	if len(mws) > 0 {
		req = req.WithMiddlewares(mws...)
	}
	if len(tools) > 0 {
		req = req.WithTools(tools...)
	}
	if params := pc.sessionParams(); len(params) > 0 {
		req = req.WithParams(params)
	}
	return req
}

// sessionParams returns the request-params map the session
// machinery needs the chat client to see — currently just the
// chat-memory conversation key. Returns nil when no session is
// bound so [buildChatRequest] can skip the WithParams call.
func (pc *ProcessContext) sessionParams() map[string]any {
	if pc.Options == nil || pc.Options.Session == nil || pc.Options.Session.ID == "" {
		return nil
	}
	return map[string]any{
		chatMemoryConversationIDKey: pc.Options.Session.ID,
	}
}

// chatMemoryConversationIDKey is the string the memory middleware
// in core/model/chat/memory reads from the request params. Kept as
// a local constant (matching the value declared at memory.ConversationIDKey)
// so agent/core doesn't import the memory package — that import
// would pull memory into every agent binary even when nobody uses
// chat sessions.
const chatMemoryConversationIDKey = "lynx:ai:model:chat:memory:conversation_id"

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
