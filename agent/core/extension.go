package core

import "context"

// Extension is the marker every plug-in capability shares. The Name
// gives each registration a stable identity used for:
//
//   - de-duplication: the runtime panics on a duplicate Name within a
//     single registration scope (PlatformConfig or ProcessOptions),
//     turning boot-time misconfiguration into a fast failure.
//   - logging / tracing: dispatch sites can attribute timings and
//     failures to a specific extension by Name.
//   - introspection: tests and ops tools list registered extensions
//     by Name without needing reflection.
//
// An empty Name is rejected. A type that wants to be plugged in
// implements Extension plus any subset of the capability interfaces
// declared below — the runtime detects each capability via a type
// assertion (mirrors net/http.ResponseWriter ↔ http.Pusher).
type Extension interface {
	Name() string
}

// ActionInterceptor wraps a single [Action] execution. The most general
// extension shape — subsumes "before-action" and "after-action" hooks
// into one around-the-call form so cross-call state lives in plain
// function locals instead of a side-channel map.
//
// Use cases: timing and metrics, audit logging with start/end
// correlation, propagation of ambient context (auth, tenancy, OTel
// baggage) into an action's goroutine, circuit-breaker / rate-limit
// (skip next() to short-circuit). Composition is onion-style: the
// first registered interceptor is the outermost layer.
//
// The runtime calls each interceptor under a panic guard; a panic
// becomes [ActionFailed] with the panic value attached.
type ActionInterceptor interface {
	Extension

	InterceptAction(
		ctx context.Context,
		process Process,
		action Action,
		next func() ActionStatus,
	) ActionStatus
}

// ToolDecorator wraps an [AgentTool] resolved by an action. Triggered
// when [ProcessContext.ActionTools] (or [ProcessContext.ResolveTools])
// turns a [ToolGroupRequirement] into concrete tools — every tool in
// the result passes through every registered decorator before reaching
// the action.
//
// Use cases: per-call tracing spans, auth / scope checks before the
// tool's underlying Call, input redaction / output filtering, retry
// on tool-level transient errors. Composition is wrap-style: the
// first registered decorator is the innermost wrap (each successive
// decorator wraps the prior result).
type ToolDecorator interface {
	Extension

	DecorateTool(
		process Process,
		action Action,
		tool AgentTool,
	) AgentTool
}

// AgentValidator runs as the last step of [Platform.Deploy], after
// the structural [ValidateAgent] check and the goal-reachability
// scan. Returning a non-nil error rejects the deployment; the
// runtime wraps the error with the validator's Name so the failure
// is attributable.
//
// Use cases: business-rule checks ("description required",
// "name must be snake_case"), cross-agent uniqueness invariants,
// dependency policy ("can't deploy without an audit listener
// registered"). Multiple validators run in registration order;
// the first error wins (fail-fast).
type AgentValidator interface {
	Extension

	ValidateAgent(agent *Agent) error
}

// GoalApprover gates the planner's goal-selection per process. The
// runtime calls every approver before each [Plan] call; a goal
// survives only when every approver returns true (conjunction —
// any false vetoes).
//
// Use cases: multi-tenant goal scoping (a user can only pursue
// goals their tenant licenses), A/B experiments, emergency
// kill-switch ("circuit-broken goals temporarily disabled").
type GoalApprover interface {
	Extension

	ApproveGoal(process Process, goal *Goal) bool
}

// BlackboardFactory supplies the [Blackboard] used by a fresh
// process. The runtime calls the last-registered factory (extensions
// override defaults). When no factory is registered, the runtime
// falls back to the built-in in-memory implementation.
//
// [ProcessOptions.Blackboard] still wins when set per-call — the
// factory only fires when the caller didn't pre-supply one.
//
// Use cases: Redis-backed blackboards for cross-process visibility,
// blackboards with audit-log mirroring, mock blackboards in tests.
type BlackboardFactory interface {
	Extension

	NewBlackboard() Blackboard
}
