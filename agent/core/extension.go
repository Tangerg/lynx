package core

import "context"

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

// ToolDecorator wraps every [AgentTool] resolved by
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
		tool AgentTool,
	) AgentTool
}

// AgentValidator runs as the last [Platform.Deploy] step (after
// [ValidateAgent] and the goal-reachability scan). A non-nil return
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
