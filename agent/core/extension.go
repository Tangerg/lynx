package core

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/tools"
)

// Extension is the marker every plug-in capability shares. Name is
// used for dedup (panic on duplicate within a registration scope),
// logging / tracing attribution, and introspection. An empty Name is
// rejected.
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
// the outermost layer. Panics in next become [ActionFailed].
type ActionMiddleware interface {
	Extension

	InterceptAction(
		ctx context.Context,
		process Process,
		action Action,
		next func() ActionStatus,
	) ActionStatus
}

// ToolDecorator wraps every [tools.Tool] resolved by
// [ProcessContext.ActionTools] / [ProcessContext.ResolveTools].
// Composition is wrap-style: first registered is innermost.
//
// Typical uses: per-call tracing, auth / scope checks, redaction,
// transient-error retry.
type ToolDecorator interface {
	Extension

	DecorateTool(
		process Process,
		action Action,
		tool tools.Tool,
	) tools.Tool
}

// AgentValidator runs as the last [Platform.Deploy] step (after
// [Agent.Validate] and the goal-reachability scan). A non-nil return
// rejects the deployment, attributed to the validator's Name.
type AgentValidator interface {
	Extension

	ValidateAgent(agent *Agent) error
}

// GoalApprover gates the planner's goal-selection: every approver
// must return true for the goal to survive (any false vetoes). Used
// for multi-tenant scoping, A/B experiments, kill-switch.
type GoalApprover interface {
	Extension

	ApproveGoal(process Process, goal *Goal) bool
}

// ChatClientProvider overrides which [chatclient.Client] a process's actions use
// for their LLM calls (via [ProcessContext.Chat] /
// [ProcessContext.PromptRunner]), instead of the single client the
// Platform was constructed with. The runtime consults registered providers
// process-scope first then platform-scope, and uses the first non-nil
// client returned; nil from all (or none registered) falls back to the
// platform's shared client.
//
// This lets one Platform serve turns against different models / providers
// chosen per process — e.g. a backend that lets each run pick its model —
// without standing up a separate Platform per model. A provider may key its
// choice on the process (read a binding / blackboard value), or simply
// carry a fixed client when registered per-process via
// [ProcessOptions.Extensions].
type ChatClientProvider interface {
	Extension

	// ChatClientFor returns the client this process should use, or nil to
	// defer to the next provider / the platform default.
	ChatClientFor(process Process) *chatclient.Client
}
