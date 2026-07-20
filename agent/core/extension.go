package core

import (
	"context"

	"github.com/Tangerg/lynx/tools"
)

// Extension is the marker every plug-in capability shares. Name is used for
// dedup, logging / tracing attribution, and introspection. Framework
// construction reports nil, empty, or duplicate registrations as errors; its
// explicit Must constructor is the panic-on-error convenience.
//
// A type that wants to be plugged in implements Extension plus any
// subset of the capability interfaces below — the runtime detects
// each capability via type assertion (mirrors
// net/http.ResponseWriter ↔ http.Pusher).
type Extension interface {
	Name() string
}

// ActionMiddleware wraps a single [Action] execution — the
// canonical around-call hook for timing, audit logging, ambient
// context propagation (auth / tenancy / OTel baggage),
// circuit-breaker / rate-limit (skip next to short-circuit).
// Composition is onion-style: the first registered interceptor is
// the outermost layer. The runtime invokes the wrapped chain at most once even
// if middleware calls next repeatedly, and converts middleware panics into
// [ActionFailed].
type ActionMiddleware interface {
	Extension

	RunAction(
		ctx context.Context,
		process ProcessView,
		action Action,
		next func() (ActionStatus, error),
	) (ActionStatus, error)
}

// ToolMiddleware wraps every [tools.Tool] resolved by
// [ProcessContext.ActionTools] / [ProcessContext.ResolveTools].
// Composition is wrap-style: first registered is innermost.
// A panic or nil result makes tool resolution fail with an error attributed to
// the middleware; it cannot leak into the host or silently remove a tool.
//
// Typical uses: per-call tracing, auth / scope checks, redaction,
// transient-error retry. Transparent decorators should structurally forward
// optional ConcurrencyKey and ReturnsDirect capabilities; stateful policies
// may deliberately omit them to narrow scheduling or continuation semantics.
type ToolMiddleware interface {
	Extension

	WrapTool(
		process ProcessView,
		action Action,
		tool tools.Tool,
	) tools.Tool
}

// AgentValidator runs as the last [Engine.Deploy] validation step after
// [Agent.Validate]. It receives the same frozen definition snapshot that the
// runtime will execute and identify durably. A non-nil return rejects the
// deployment, attributed to the validator's Name.
type AgentValidator interface {
	Extension

	Validate(agent *Agent) error
}

// GoalApprover gates the planner's goal-selection: every approver
// must return true for the goal to survive (any false vetoes). Used
// for multi-tenant scoping, A/B experiments, kill-switch.
// A panic is an extension failure, not a veto, and fails the process.
type GoalApprover interface {
	Extension

	Approve(process ProcessView, goal *Goal) bool
}

// ChatProvider overrides which provider-neutral chat capabilities a process's
// actions use for their LLM calls (via [ProcessContext.Chat] /
// [ProcessContext.Prompt]), instead of the engine's default model.
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
// A panic fails capability resolution and is attributed to the provider.
type ChatProvider interface {
	Extension

	// Chat returns the capability this process should use. A nil Model
	// defers to the next provider or engine default and must be accompanied
	// by a nil Streamer; streaming is an optional capability of a selected
	// synchronous model, never an independent routing result.
	Chat(process ProcessView) ChatCapability
}
