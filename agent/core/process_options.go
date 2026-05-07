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

	Budget         Budget
	ProcessControl ProcessControl

	OutputChannel OutputChannel

	// PlannerType selects which planner the runtime requests from the
	// platform's PlannerFactory. Only [PlannerGOAP] is wired today;
	// [PlannerUtility] is reserved for a future reward-based planner.
	PlannerType PlannerType

	ProcessType ProcessType

	// Extensions are session-scoped plug-ins active for the lifetime of
	// this single process. They merge with platform-scoped extensions at
	// dispatch time — process extensions take inner / higher priority
	// (e.g. a process-scope IDGenerator overrides the platform default;
	// a process-scope ActionInterceptor sits inside any platform-scope
	// interceptor in the onion chain). Within Extensions, each
	// [Extension.Name] must be unique; the runtime returns an error
	// from RunAgent / StartAgent / ContinueProcess when this constraint
	// is violated. Process-scope Names may collide with platform-scope
	// Names — that's the explicit override mechanism.
	Extensions []Extension
}

// ApplyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver. Idempotent — safe to call repeatedly.
// The runtime invokes this on every [ProcessOptions] it receives, so
// users normally don't need to call it themselves.
//
// PlannerType and ProcessType are int8 enums whose zero value already
// matches the desired default ([PlannerGOAP] / [ProcessSequential]), so they
// need no explicit handling.
func (o *ProcessOptions) ApplyDefaults() {
	if o.Budget == (Budget{}) {
		o.Budget = DefaultBudget()
	}
	if o.OutputChannel == nil {
		o.OutputChannel = DevNullOutputChannel
	}
	// Wire Budget into the early-termination check by default — mirrors
	// embabel's ProcessOptions(processControl = ProcessControl(... =
	// budget.earlyTerminationPolicy())). A zero ProcessOptions{} now
	// actually enforces DefaultBudget; callers who want unlimited can
	// pass a custom EarlyTerminationPolicy that ignores Budget.
	if o.ProcessControl.EarlyTerminationPolicy == nil {
		o.ProcessControl.EarlyTerminationPolicy = o.Budget.EarlyTerminationPolicy()
	}
}

// Budget caps cumulative LLM spend (USD), action invocations, and total
// tokens for one process. Budget is enforced via [BudgetPolicy], which
// [ProcessOptions.ApplyDefaults] installs as the default
// [ProcessControl.EarlyTerminationPolicy] when none is supplied — so a
// zero-options caller gets the [DefaultBudget] limits automatically. To
// disable, set a custom EarlyTerminationPolicy.
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

// EarlyTerminationPolicy returns the policy that enforces this Budget.
// Mirrors embabel's Budget.earlyTerminationPolicy(): a single composite
// check on cost / tokens / actions. [ProcessOptions.ApplyDefaults] uses
// this to wire Budget into ProcessControl when the caller didn't supply
// an explicit policy.
func (b Budget) EarlyTerminationPolicy() EarlyTerminationPolicy {
	return BudgetPolicy{Budget: b}
}

// ProcessControl wraps the early-termination policy. Wrapper kept (not
// lifted to ProcessOptions top level) so future tick-control knobs can
// be added without churning the ProcessOptions field set.
type ProcessControl struct {
	EarlyTerminationPolicy EarlyTerminationPolicy
}
