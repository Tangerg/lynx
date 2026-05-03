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
	Identities     Identities
	Blackboard     Blackboard
	Verbosity      Verbosity
	Budget         Budget
	ProcessControl ProcessControl
	Prune          bool
	OutputChannel  OutputChannel
	PlannerType    PlannerType
	ProcessType    ProcessType
}

// ApplyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver. Idempotent — safe to call repeatedly.
// The runtime invokes this on every [ProcessOptions] it receives, so
// users normally don't need to call it themselves.
//
// PlannerType and ProcessType are int8 enums whose zero value already
// matches the desired default ([PlannerGOAP] / [ProcessSimple]), so they
// need no explicit handling.
func (o *ProcessOptions) ApplyDefaults() {
	if o.Budget == (Budget{}) {
		o.Budget = DefaultBudget()
	}
	if o.OutputChannel == nil {
		o.OutputChannel = DevNullOutputChannel
	}
}

// Verbosity controls instrumentation visible to humans. None of these fields
// affect program behavior — they're hints to listeners and the LLM-debug UI.
type Verbosity struct {
	ShowPrompts      bool
	ShowLLMResponses bool
	Debug            bool
	ShowPlanning     bool
}

// Budget caps cumulative LLM spend (USD), action invocations, and total
// tokens for one process. The runtime checks Budget at tick boundaries and
// transitions to StatusTerminated when exceeded.
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

// ProcessControl carries throttling knobs and the early-termination policy.
type ProcessControl struct {
	EarlyTerminationPolicy EarlyTerminationPolicy
	ToolDelay              time.Duration
	OperationDelay         time.Duration
}

// Identities pairs the user this process is acting on behalf of with the
// identity used for impersonation/audit. Both are optional.
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
