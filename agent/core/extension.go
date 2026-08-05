package core

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

// Extension is the marker every plug-in capability shares. Name is read once
// and frozen at its registration boundary, so a later change to what Name
// returns cannot alter how an already-registered value is identified.
//
// A type that wants to be plugged in implements Extension plus any
// subset of the capability interfaces below — the runtime detects
// each capability via type assertion (mirrors
// net/http.ResponseWriter ↔ http.Pusher).
//
// Registered instances may be called concurrently by different processes.
// Implementations own synchronization for their mutable state; the runtime does
// not serialize, retry, or otherwise coordinate extension calls. Each
// capability documents whether it is valid at engine scope, process scope, or
// both. Registering a value with no capability valid for that scope is an error.
type Extension interface {
	Name() string
}

// ActionMiddleware wraps one action execution without receiving the executable
// [Action] itself. The descriptor is an inert definition snapshot; next is the
// middleware's only execution authority.
//
// Skipping next short-circuits the action. Composition is onion-style, the
// first registered interceptor outermost, and the wrapped chain runs at most
// once however many times a middleware calls next. A middleware panic becomes
// [ActionFailed]. Valid at engine and process scope.
type ActionMiddleware interface {
	Extension

	RunAction(
		ctx context.Context,
		process ProcessView,
		action ActionDescriptor,
		next func() (ActionStatus, error),
	) (ActionStatus, error)
}

// ToolMiddleware wraps every [tool.Tool] resolved by
// [ProcessContext.ActionTools].
// Composition is wrap-style: first registered is innermost.
// A panic or nil result makes tool resolution fail with an error attributed to
// the middleware; it cannot leak into the host or silently remove a tool.
//
// A wrapper declares [tool.WrappingTool], so the tool it stands in for keeps
// every optional capability it declared — one method, whatever the set of
// capabilities grows to. Re-implementing capabilities one by one silently
// drops the ones a wrapper forgot. A policy that means to narrow scheduling or
// continuation semantics declares the capability itself, which takes precedence
// over the wrapped tool's, or omits Unwrap to hide the tool entirely.
// Valid at engine and process scope.
type ToolMiddleware interface {
	Extension

	WrapTool(
		process ProcessView,
		action ActionDescriptor,
		tool tool.Tool,
	) tool.Tool
}

// AgentValidator runs as the engine's last deploy-time validation step after
// [Agent.Validate]. It receives an inert descriptor of the frozen definition,
// not executable actions or conditions. A non-nil return rejects the
// deployment, attributed to the validator's Name. Valid only at engine scope.
type AgentValidator interface {
	Extension

	Validate(agent AgentDescriptor) error
}

// GoalApprover gates the planner's goal-selection: every approver
// must return true for the goal to survive (any false vetoes). Used
// for multi-tenant scoping, A/B experiments, kill-switch.
// A panic is an extension failure, not a veto, and fails the process. Valid at
// engine and process scope.
type GoalApprover interface {
	Extension

	Approve(process ProcessView, goal GoalDescriptor) bool
}

// ChatProvider overrides which provider-neutral chat capabilities a process's
// managed Interact and Prompt calls use instead of the engine's default model.
// The runtime consults registered providers process-scope first then
// engine-scope, and uses the first capability with a non-nil Model; nil
// from all (or none registered) falls back to the engine capability.
//
// This lets one Engine serve turns against different models / providers
// chosen per process — e.g. a backend that lets each run pick its model —
// without standing up a separate Engine per model. A provider may key its
// choice on the process (read a binding / blackboard value), or simply
// carry fixed model protocols when registered per-process via
// [ProcessOptions.Extensions].
// A panic fails capability resolution and is attributed to the provider. Valid
// at engine and process scope.
type ChatProvider interface {
	Extension

	// Chat returns the capability this process should use. A nil Model
	// defers to the next provider or engine default and must be accompanied
	// by a nil Streamer; streaming is an optional capability of a selected
	// synchronous model, never an independent routing result.
	Chat(process ProcessView) ChatCapability
}

// InteractionCostProjector maps one managed model response to the host's
// non-negative cost unit. Runtime owns token/model-call counters and invokes
// only the first projector in resolver order (process scope before engine
// scope). A panic or invalid result fails the interaction. Valid at engine and
// process scope.
type InteractionCostProjector interface {
	Extension

	ProjectInteractionCost(
		ctx context.Context,
		process ProcessView,
		response *chat.Response,
	) (float64, error)
}
