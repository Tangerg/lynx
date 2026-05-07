package core

import "time"

// ProcessOptions is the per-process configuration bundle. Pass a zero
// ProcessOptions{} when defaults suffice — the runtime calls
// [ProcessOptions.ApplyDefaults] before use, so unset fields receive
// their conceptual defaults (Budget, PlannerType, ProcessType,
// OutputChannel).
//
// Choosing a struct over the functional-options pattern keeps defaults
// + validation in one place ([ApplyDefaults]) and avoids polluting the
// package namespace with ~10 `With…` constructors. Direct struct-
// literal init is the intended ergonomics.
type ProcessOptions struct {
	// Identities: TODO(future) — wire into audit logs and RBAC checks
	// via a future authorization Extension. Today the framework just
	// stores them; integration code may consume.
	Identities Identities

	Blackboard Blackboard

	// Verbosity: TODO(future) — currently observational hints stored
	// for integration LLM listeners; no framework branching today.
	Verbosity Verbosity

	Budget         Budget
	ProcessControl ProcessControl

	// Prune: TODO(future) — referenced by plan.Planner.Prune doc but no
	// runtime path consumes it today. Reserved for "drop unreachable
	// actions on cold start" mode.
	Prune bool

	OutputChannel OutputChannel

	// PlannerType: TODO(future) — only [PlannerGOAP] is wired in the
	// default factory. [PlannerUtility] is reserved for a reward-based
	// planner.
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

// Verbosity controls instrumentation visible to humans. None of these
// fields affect program behavior — they're hints to listeners and the
// LLM-debug UI.
//
// TODO(future): consumed by integration LLM listeners (e.g. a slog
// adapter that gates DEBUG-level prompt dumps on ShowPrompts). Framework
// itself never reads these fields.
type Verbosity struct {
	ShowPrompts      bool
	ShowLLMResponses bool
	Debug            bool
	ShowPlanning     bool
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

// ProcessControl carries throttling knobs and the early-termination policy.
type ProcessControl struct {
	EarlyTerminationPolicy EarlyTerminationPolicy

	// ToolDelay / OperationDelay: TODO(future) — placeholder for global
	// throttling between tool invocations / between tick operations.
	// Not consumed by the framework today; integration code can read
	// them but the runtime doesn't sleep on them.
	ToolDelay      time.Duration
	OperationDelay time.Duration
}

// Identities pairs the user this process is acting on behalf of with the
// identity used for impersonation/audit. Both are optional.
//
// TODO(future): wire into audit log + RBAC checks via an Extension.
// Today the framework just stores; nothing reads.
type Identities struct {
	ForUser *User
	RunAs   *User
}

// User is the lightweight identity model — just enough to attach to events
// and audit logs.
type User struct {
	ID       string
	Name     string
	Metadata map[string]any
}
