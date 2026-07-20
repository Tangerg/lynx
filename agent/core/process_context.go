package core

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ChatCapability is the provider-neutral model surface available to an action.
// Model is required; Streamer is optional. Runtime composition may back both
// with one value implementing both interfaces, but core depends only on
// provider-neutral chat protocols.
type ChatCapability struct {
	Model    chat.Model
	Streamer chat.Streamer
}

// ModelAttribution enriches framework-owned usage records with information the
// provider-neutral response cannot know. Tokens, model, action, duration, and
// timestamp remain runtime-owned.
type ModelAttribution struct {
	Provider string
	CostUSD  float64
}

type ModelAttributionFunc func(response *chat.Response) ModelAttribution

// Interaction is one framework-managed model/tool exchange. ID is optional;
// runtime derives a stable owner from the process, action, and request when it
// is empty.
type Interaction struct {
	ID        string
	Model     chat.Model
	Request   *chat.Request
	Tools     interaction.ToolResolver
	Limits    interaction.Limits
	Observe   interaction.Observer
	Attribute ModelAttributionFunc
}

// ProcessContextConfig is the narrow bridge used by the sibling runtime
// package to assemble one action capability object. It is not general user
// configuration; fields mirror the runtime-owned process scope and callbacks
// without exporting secondary hook-bundle types.
type ProcessContextConfig struct {
	Process      ProcessView
	Control      ProcessControl
	Usage        UsageRecorder
	Blackboard   Blackboard
	Session      *Session
	Dependencies *Dependencies

	// Chat resolves the process-scoped model/stream capabilities. nil means no
	// model was configured.
	Chat func() (ChatCapability, error)

	// MaxToolRounds is the resolved process-level Prompt limit.
	MaxToolRounds int

	// Emit is invoked by [ProcessContext.Emit]; nil makes Emit a
	// no-op. The runtime supplies a closure that fans the event out to the
	// engine's multicast listener.
	Emit func(context.Context, any)

	// ResolveTools is invoked by [ProcessContext.ResolveTools]; nil
	// makes ResolveTools return (nil, nil). The runtime supplies a
	// closure backed by the engine's [ToolGroupResolver].
	ResolveTools func(context.Context, []ToolGroupRequirement) ([]tools.Tool, error)

	// RunInteraction executes framework-managed model/tool control flow.
	RunInteraction func(context.Context, Interaction) (interaction.Result, error)

	// ToolCallCancel registers a cancel func and returns a release
	// closure — single function rather than a register/clear pair so
	// callers can't mismatch them.
	ToolCallCancel func(context.CancelFunc) (release func())

	// ActionToolGroups carries the currently-executing action's declared
	// [ToolGroupRequirement]s, so [ProcessContext.ActionTools] can
	// resolve them without the action body having to re-state role names.
	ActionToolGroups []ToolGroupRequirement
}

// ProcessContext is the only thing handed to an [Action.Execute] call.
// Every dependency the action might need lives behind a method here so
// future refactors don't ripple through every action body.
//
// Field grouping mirrors [ProcessContextConfig]: action-facing capabilities up
// top, engine-wired hooks in the middle (held privately so callers go
// through the typed methods), and private per-action lifecycle state at the
// bottom.
type ProcessContext struct {
	process      ProcessView
	blackboard   Blackboard
	dependencies *Dependencies
	session      SessionInfo
	hasSession   bool

	// Engine-wired hooks. Private so action bodies go through
	// the typed methods (Chat / Emit / ResolveTools / ...) instead
	// of touching the underlying client / closure directly.
	chat           func() (ChatCapability, error)
	maxToolRounds  int
	emit           func(context.Context, any)
	resolveTools   func(context.Context, []ToolGroupRequirement) ([]tools.Tool, error)
	runInteraction func(context.Context, Interaction) (interaction.Result, error)
	toolCallCancel func(context.CancelFunc) (release func())
	control        ProcessControl
	usage          UsageRecorder

	actionToolGroups []ToolGroupRequirement

	// suspended flips when the action calls [Suspend]; the
	// typed-action wrapper reads it to return ActionWaiting. Per-tick
	// (fresh ProcessContext each invocation), so no reset needed.
	suspended bool

	parallelBranch bool
}

// NewProcessContext assembles a ProcessContext from config. Used by the
// runtime once per tick; users don't construct ProcessContexts themselves.
func NewProcessContext(config ProcessContextConfig) *ProcessContext {
	dependencies := config.Dependencies
	if dependencies == nil {
		dependencies = NewDependencies()
	}
	session, hasSession := config.Session.info()
	return &ProcessContext{
		process:          config.Process,
		control:          config.Control,
		usage:            config.Usage,
		blackboard:       config.Blackboard,
		dependencies:     dependencies,
		session:          session,
		hasSession:       hasSession,
		chat:             config.Chat,
		maxToolRounds:    config.MaxToolRounds,
		actionToolGroups: config.ActionToolGroups,
		emit:             config.Emit,
		resolveTools:     config.ResolveTools,
		runInteraction:   config.RunInteraction,
		toolCallCancel:   config.ToolCallCancel,
	}
}

// Process returns the read-only running process view.
func (pc *ProcessContext) Process() ProcessView {
	if pc == nil {
		return nil
	}
	return pc.process
}

// Blackboard returns the mutable action-local process memory.
func (pc *ProcessContext) Blackboard() Blackboard {
	if pc == nil {
		return nil
	}
	return pc.blackboard
}

// Dependencies returns the action dependency scope.
func (pc *ProcessContext) Dependencies() *Dependencies {
	if pc == nil {
		return nil
	}
	return pc.dependencies
}

// Session returns an immutable identity/audit snapshot for the process's
// multi-turn session. ok is false for an ordinary process run. Host-owned
// mutable Session.Metadata is intentionally not exposed to actions.
func (pc *ProcessContext) Session() (session SessionInfo, ok bool) {
	if pc == nil || !pc.hasSession {
		return SessionInfo{}, false
	}
	return pc.session, true
}

// ForParallelBranch returns a sibling-safe copy of pc for a goroutine running
// concurrently with other branches of the SAME action — the workflow fan-out
// builders (ScatterGather / Consensus / Parallel) hand one to each generator.
//
// It shares the read-only ProcessView, usage recorder, and safe output
// channel, but forks Blackboard and action-dependency state from the same
// pre-branch snapshot. Branch writes, conditions, and dependency registrations
// are local and discarded; the workflow commits only returned values in
// declaration order. Lifecycle control and managed interaction are disabled
// because one Process cannot own multiple competing suspension/termination
// continuations.
func (pc *ProcessContext) ForParallelBranch() *ProcessContext {
	if pc == nil {
		return nil
	}
	branch := *pc
	if pc.blackboard != nil {
		branch.blackboard = pc.blackboard.Clone()
	}
	if pc.dependencies != nil {
		branch.dependencies = pc.dependencies.Child()
	}
	branch.control = nil
	branch.chat = nil
	branch.runInteraction = func(context.Context, Interaction) (interaction.Result, error) {
		return interaction.Result{}, ErrParallelBranchControl
	}
	branch.toolCallCancel = nil
	branch.suspended = false
	branch.parallelBranch = true
	return &branch
}

// Chat returns provider-neutral model and stream capabilities scoped to this process.
func (pc *ProcessContext) Chat() (ChatCapability, error) {
	if pc == nil || pc.chat == nil {
		if pc != nil && pc.parallelBranch {
			return ChatCapability{}, ErrParallelBranchControl
		}
		return ChatCapability{}, errors.New("agent.ProcessContext.Chat: no chat model configured on the engine")
	}
	capability, err := pc.chat()
	if err != nil {
		return ChatCapability{}, err
	}
	if capability.Model == nil {
		return ChatCapability{}, errors.New("agent.ProcessContext.Chat: runtime resolved a nil chat model")
	}
	return capability, nil
}

// Interact runs a complete framework-managed model/tool interaction and
// preserves its terminal event.
func (pc *ProcessContext) Interact(ctx context.Context, input Interaction) (interaction.Result, error) {
	if pc == nil || pc.runInteraction == nil {
		return interaction.Result{}, errors.New("agent.ProcessContext.Interact: managed interaction is not configured")
	}
	return pc.runInteraction(contextOrBackground(ctx), input)
}

// Suspend parks one durable continuation on the current process.
func (pc *ProcessContext) Suspend(ctx context.Context, suspension interaction.Suspension) (ActionStatus, error) {
	if pc == nil || pc.control == nil {
		if pc != nil && pc.parallelBranch {
			return ActionFailed, ErrParallelBranchControl
		}
		return ActionFailed, errors.New("agent: process context has no lifecycle control")
	}
	status, err := pc.control.Suspend(contextOrBackground(ctx), suspension)
	if err != nil {
		return status, err
	}
	if status == ActionWaiting {
		pc.suspended = true
	}
	return status, nil
}

// TerminateAgent requests process termination at the next tick boundary.
func (pc *ProcessContext) TerminateAgent(reason string) error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateAgent(reason)
	return nil
}

// TerminateAction requests re-planning without terminating the process.
func (pc *ProcessContext) TerminateAction(reason string) error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateAction(reason)
	return nil
}

// TerminateToolCall cancels the process's registered in-flight tool call.
func (pc *ProcessContext) TerminateToolCall() error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateToolCall()
	return nil
}

// Emit delivers an event to the runtime's listeners.
func (pc *ProcessContext) Emit(ctx context.Context, event any) {
	if pc.emit != nil {
		pc.emit(contextOrBackground(ctx), event)
	}
}

// ResolveTools resolves unprivileged tool roles through the engine-configured resolver.
func (pc *ProcessContext) ResolveTools(ctx context.Context, roles ...string) ([]tools.Tool, error) {
	if pc.resolveTools == nil {
		return nil, nil
	}
	requirements := make([]ToolGroupRequirement, len(roles))
	for i, role := range roles {
		requirements[i] = RequireToolGroup(role)
	}
	return pc.resolveTools(contextOrBackground(ctx), requirements)
}

// ActionTools resolves the tool groups declared by the current action.
func (pc *ProcessContext) ActionTools(ctx context.Context) ([]tools.Tool, error) {
	if pc.resolveTools == nil || len(pc.actionToolGroups) == 0 {
		return nil, nil
	}
	return pc.resolveTools(contextOrBackground(ctx), pc.actionToolGroups)
}

// ToolCallContext derives a child context cancellable through TerminateToolCall.
// The returned cancel function also unregisters the runtime callback and must
// be called when the tool invocation finishes.
func (pc *ProcessContext) ToolCallContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(contextOrBackground(parent))
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

// RecordUsage attributes aggregate model usage to the running process.
func (pc *ProcessContext) RecordUsage(ctx context.Context, cost float64, tokens int) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordUsage(contextOrBackground(ctx), cost, tokens)
}

// RecordModelCall attributes one model call to the running process.
func (pc *ProcessContext) RecordModelCall(ctx context.Context, call ModelCall) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordModelCall(contextOrBackground(ctx), call)
}

// RecordEmbeddingCall attributes one embedding call to the running process.
func (pc *ProcessContext) RecordEmbeddingCall(ctx context.Context, call EmbeddingCall) {
	if pc == nil || pc.usage == nil {
		return
	}
	pc.usage.RecordEmbeddingCall(contextOrBackground(ctx), call)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// ErrParallelBranchControl reports lifecycle or managed-interaction use from
// a workflow branch. Use an isolated child Process when a parallel unit needs
// suspension, termination, or its own model/tool lifecycle.
var ErrParallelBranchControl = errors.New("agent: parallel workflow branch cannot control process lifecycle")
