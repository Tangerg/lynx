package core

// ProcessOptions is the per-process configuration bundle. Pass a zero
// ProcessOptions{} when defaults suffice — the runtime calls
// [ProcessOptions.ApplyDefaults] before use, so unset fields receive
// their conceptual defaults.
//
// Choosing a struct over the functional-options pattern keeps defaults
// + validation in one place ([ApplyDefaults]) and avoids polluting the
// package namespace with ~10 `With…` constructors. Direct struct-
// literal init is the intended ergonomics. Cross-cutting concerns
// (audit, verbosity, throttling, RBAC) belong on extensions registered
// via [Extensions]; ProcessOptions itself stays minimal.
type ProcessOptions struct {
	Blackboard Blackboard

	// Budget caps cumulative LLM spend (USD), action invocations, and
	// total tokens for this process. The runtime always checks a
	// Budget-derived [BudgetPolicy] implicitly each tick; additional
	// [EarlyTerminationPolicy] extensions can be registered via
	// Extensions (OR semantics — any policy triggers termination).
	Budget Budget

	OutputChannel OutputChannel

	ProcessType ProcessType

	// Session optionally binds this process to a multi-turn
	// conversation. ProcessContext binds the session ID to call context so the
	// history middleware loads and persists history keyed by [Session.ID]
	// without serializing runtime scope into chat.Request.
	//
	// Typically set via [Platform.RunInSession]; the runtime fills
	// the field and refreshes [Session.UpdatedAt] on every dispatch.
	Session *Session

	// Extensions are session-scoped plug-ins active for the lifetime of
	// this single process. They merge with platform-scoped extensions at
	// dispatch time — process extensions take inner / higher priority
	// (e.g. a process-scope IDGenerator overrides the platform default;
	// a process-scope ActionMiddleware sits inside any platform-scope
	// interceptor in the onion chain). Within Extensions, each
	// [Extension.Name] must be unique; the runtime returns an error
	// from RunAgent / StartAgent / ContinueProcess when this constraint
	// is violated. Process-scope Names may collide with platform-scope
	// Names — that's the explicit override mechanism.
	Extensions []Extension

	// Guardrails, when non-nil, overrides the platform-level guardrails
	// for this process. nil means "use the platform default". Set it to
	// inject per-process chat middleware (tool loop, history) so agent/core
	// doesn't need to import middleware implementations.
	Guardrails *Guardrails
}

// ApplyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver. Idempotent — safe to call repeatedly.
// The runtime invokes this on every [ProcessOptions] it receives, so
// users normally don't need to call it themselves.
//
// ProcessType is an int8 enum whose zero value already matches the
// desired default ([ProcessSequential]), so it needs no explicit
// handling.
func (o *ProcessOptions) ApplyDefaults() {
	if o.Budget == (Budget{}) {
		o.Budget = DefaultBudget()
	}
	if o.OutputChannel == nil {
		o.OutputChannel = DevNullOutputChannel
	}
}

// Budget caps cumulative LLM spend (USD), action invocations, and total
// tokens for one process. Budget is enforced via [BudgetPolicy], which
// the runtime checks implicitly each tick — so a zero-options caller
// gets the [DefaultBudget] limits automatically. Additional policies
// (DLP, rate-limit, custom guardrails) can be registered as
// [EarlyTerminationPolicy] extensions; all are OR-composed.
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

// EarlyTerminationPolicy returns a [BudgetPolicy] that enforces this
// Budget. Useful when callers want to register the budget check as
// an explicit extension alongside other policies, or to construct
// the BudgetPolicy without typing the struct literal.
func (b Budget) EarlyTerminationPolicy() EarlyTerminationPolicy {
	return BudgetPolicy{Budget: b}
}
