package core

// ProcessOptions is the per-process configuration bundle. Pass a zero
// ProcessOptions{} when defaults suffice; the runtime normalizes unset fields.
//
// Choosing a struct over the functional-options pattern keeps defaults
// and validation in one place and avoids polluting the
// package namespace with ~10 `With…` constructors. Direct struct-
// literal init is the intended ergonomics. Cross-cutting concerns
// (audit, verbosity, throttling, RBAC) belong on extensions registered
// via [Extensions]; ProcessOptions itself stays minimal.
type ProcessOptions struct {
	Blackboard Blackboard

	// Budget caps cumulative LLM spend (USD), action invocations, and
	// total tokens for this process. The runtime always checks a
	// Budget-derived [BudgetPolicy] implicitly each tick; additional
	// [StopPolicy] extensions can be registered via
	// Extensions (OR semantics — any policy triggers termination).
	Budget Budget

	// Dependencies is an optional process scope created from
	// [runtime.Engine.Dependencies] by calling [Dependencies.Child]. The runtime
	// freezes it when the process starts and creates a fresh action child for
	// each execution. nil creates an empty process scope over the engine
	// dependencies automatically.
	//
	// The parent relationship is validated at runtime so an unrelated dependency
	// tree cannot silently bypass engine composition.
	Dependencies *Dependencies

	// Session optionally binds this process to a multi-turn
	// conversation. ProcessContext binds the session ID to call context so the
	// history middleware loads and persists history keyed by [Session.ID]
	// without serializing runtime scope into chat.Request.
	//
	// Typically set via [Engine.RunInSession]; the runtime fills
	// the field and refreshes [Session.UpdatedAt] on every dispatch.
	Session *Session

	// Extensions are session-scoped plug-ins active for the lifetime of
	// this single process. They merge with engine-scoped extensions at
	// dispatch time — process extensions take inner / higher priority
	// (e.g. a process-scope IDGenerator overrides the engine default;
	// a process-scope ActionMiddleware sits inside any engine-scope
	// interceptor in the onion chain). Within Extensions, each
	// [Extension.Name] must be unique; the runtime returns an error
	// from Run / Start / Continue when this constraint
	// is violated. Process-scope Names may collide with engine-scope
	// Names — that's the explicit override mechanism.
	Extensions []Extension

	// Guardrails, when non-nil, overrides the engine-level guardrails
	// for this process. nil means "use the engine default". Set it to
	// inject per-process chat middleware (tool loop, history) so agent/core
	// doesn't need to import middleware implementations.
	Guardrails *ChatGuardrails
}

// Budget caps cumulative LLM spend (USD), action invocations, and total
// tokens for one process. Budget is enforced via [BudgetPolicy], which
// the runtime checks implicitly each tick — so a zero-options caller
// gets the [DefaultBudget] limits automatically. Additional policies
// (DLP, rate-limit, custom guardrails) can be registered as
// [StopPolicy] extensions; all are OR-composed.
type Budget struct {
	CostLimit   float64
	ActionLimit int
	TokenLimit  int
}

// DefaultBudget is the baseline that applies when callers don't supply one —
// ~$2 cap, 50 actions, 1M tokens. The numbers are deliberately generous;
// production deployments tune them per-tenant.
func DefaultBudget() Budget {
	return Budget{CostLimit: 2.0, ActionLimit: 50, TokenLimit: 1_000_000}
}
