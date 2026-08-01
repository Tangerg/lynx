package core

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

// ChatCapability is the provider-neutral model surface supplied to the
// runtime. Model is required; Streamer is optional. Runtime composition may
// back both with one value implementing both interfaces, but actions access
// them only through the managed Interact and Prompt boundaries.
type ChatCapability struct {
	Model    chat.Model
	Streamer chat.Streamer
}

// Interaction is one framework-managed model/tool exchange. ID is optional;
// runtime derives a stable owner from the process, action, and request when it
// is empty. Stream receives raw provider deltas when set; runtime resolves and
// scopes the process Streamer, accumulates the final response, and retains
// ownership of accounting and lifecycle boundaries.
type Interaction struct {
	ID      string
	Request *chat.Request
	Tools   interaction.ToolResolver
	Limits  interaction.Limits
	Stream  func(*chat.Response)
}

// ProcessContextConfig is the runtime SPI for assembling one action capability
// object. Runtime implementations and action tests provide only the
// capabilities they support; application execution policy does not belong
// here.
type ProcessContextConfig struct {
	Process      ProcessView
	Control      ProcessControl
	Blackboard   Blackboard
	Dependencies *Dependencies

	// MaxToolRounds is the resolved process-level tool-round limit.
	MaxToolRounds int

	// ActionTools backs [ProcessContext.ActionTools]. The runtime supplies a
	// closure backed by the engine's [ToolGroupResolver].
	ActionTools func(context.Context, []string) ([]tools.Tool, error)

	// RunInteraction executes framework-managed model/tool control flow.
	RunInteraction func(context.Context, Interaction) (interaction.Result, error)

	// ToolCallCancel registers a cancel func and returns a release
	// closure — single function rather than a register/clear pair so
	// callers can't mismatch them.
	ToolCallCancel func(context.CancelFunc) (release func())

	// ActionToolGroups carries the currently-executing action's declared
	// abstract tool roles, so [ProcessContext.ActionTools] can
	// resolve them without the action body having to re-state role names.
	ActionToolGroups []string
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

	// Engine-wired hooks. Private so action bodies go through
	// the typed methods instead
	// of touching the underlying client / closure directly.
	maxToolRounds  int
	actionTools    func(context.Context, []string) ([]tools.Tool, error)
	runInteraction func(context.Context, Interaction) (interaction.Result, error)
	toolCallCancel func(context.CancelFunc) (release func())
	control        ProcessControl

	actionToolGroups []string

	// suspended flips when the action calls [Suspend]; the
	// typed-action wrapper reads it to return ActionWaiting. Per-tick
	// (fresh ProcessContext each invocation), so no reset needed.
	suspended bool
}

// NewProcessContext assembles a ProcessContext for a runtime tick or an
// isolated action test.
func NewProcessContext(config ProcessContextConfig) *ProcessContext {
	dependencies := config.Dependencies
	if dependencies == nil {
		dependencies = NewDependencies()
	}
	return &ProcessContext{
		process:          config.Process,
		control:          config.Control,
		blackboard:       config.Blackboard,
		dependencies:     dependencies,
		maxToolRounds:    config.MaxToolRounds,
		actionToolGroups: config.ActionToolGroups,
		actionTools:      config.ActionTools,
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

// Interact runs a complete framework-managed model/tool interaction and
// preserves its terminal event.
func (pc *ProcessContext) Interact(ctx context.Context, input Interaction) (interaction.Result, error) {
	if pc == nil || pc.runInteraction == nil {
		return interaction.Result{}, errors.New("agent.ProcessContext.Interact: managed interaction is not configured")
	}
	return pc.runInteraction(contextOrBackground(ctx), input)
}

// Suspend parks one snapshot-compatible continuation on the current process.
func (pc *ProcessContext) Suspend(ctx context.Context, suspension interaction.Suspension) (ActionStatus, error) {
	control, err := pc.lifecycleControl()
	if err != nil {
		return ActionFailed, err
	}
	status, err := control.Suspend(contextOrBackground(ctx), suspension)
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
	control, err := pc.lifecycleControl()
	if err != nil {
		return err
	}
	control.TerminateAgent(reason)
	return nil
}

// TerminateAction requests re-planning without terminating the process.
func (pc *ProcessContext) TerminateAction(reason string) error {
	control, err := pc.lifecycleControl()
	if err != nil {
		return err
	}
	control.TerminateAction(reason)
	return nil
}

// TerminateToolCall cancels the process's registered in-flight tool call.
func (pc *ProcessContext) TerminateToolCall() error {
	control, err := pc.lifecycleControl()
	if err != nil {
		return err
	}
	control.TerminateToolCall()
	return nil
}

func (pc *ProcessContext) lifecycleControl() (ProcessControl, error) {
	if pc != nil && pc.control != nil {
		return pc.control, nil
	}
	return nil, ErrLifecycleControlUnavailable
}

// ActionTools resolves the tool groups declared by the current action.
func (pc *ProcessContext) ActionTools(ctx context.Context) ([]tools.Tool, error) {
	if pc.actionTools == nil || len(pc.actionToolGroups) == 0 {
		return nil, nil
	}
	return pc.actionTools(contextOrBackground(ctx), pc.actionToolGroups)
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

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// ErrLifecycleControlUnavailable reports lifecycle use on a ProcessContext
// that was not assembled by the runtime.
var ErrLifecycleControlUnavailable = errors.New("agent: process lifecycle control is unavailable")
