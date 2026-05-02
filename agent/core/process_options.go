package core

import "time"

// ProcessOptions is the per-process configuration bundle. Construction goes
// through NewProcessOptions(opts...) to ensure defaults are applied — direct
// struct-literal init risks "all-zero Budget" footguns.
type ProcessOptions struct {
	ContextID       string
	Identities      Identities
	Blackboard      Blackboard
	Verbosity       Verbosity
	Budget          Budget
	ProcessControl  ProcessControl
	Prune           bool
	OutputChannel   OutputChannel
	PlannerType     PlannerType
	ProcessType     ProcessType
	ToolCallContext map[string]any
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

// ProcessOptionFunc is the option type for NewProcessOptions.
type ProcessOptionFunc func(*ProcessOptions)

// NewProcessOptions applies sensible defaults then runs caller-supplied
// options. Callers should always go through this function rather than
// initializing the struct manually.
func NewProcessOptions(opts ...ProcessOptionFunc) *ProcessOptions {
	po := &ProcessOptions{
		Budget:        DefaultBudget(),
		PlannerType:   PlannerGOAP,
		ProcessType:   ProcessSimple,
		OutputChannel: DevNullOutputChannel,
	}
	for _, opt := range opts {
		opt(po)
	}
	return po
}

func WithContextID(id string) ProcessOptionFunc       { return func(p *ProcessOptions) { p.ContextID = id } }
func WithIdentities(i Identities) ProcessOptionFunc   { return func(p *ProcessOptions) { p.Identities = i } }
func WithExistingBlackboard(b Blackboard) ProcessOptionFunc {
	return func(p *ProcessOptions) { p.Blackboard = b }
}
func WithVerbosity(v Verbosity) ProcessOptionFunc     { return func(p *ProcessOptions) { p.Verbosity = v } }
func WithBudget(b Budget) ProcessOptionFunc           { return func(p *ProcessOptions) { p.Budget = b } }
func WithProcessControl(c ProcessControl) ProcessOptionFunc {
	return func(p *ProcessOptions) { p.ProcessControl = c }
}
func WithPrune(b bool) ProcessOptionFunc              { return func(p *ProcessOptions) { p.Prune = b } }
func WithOutputChannel(c OutputChannel) ProcessOptionFunc {
	return func(p *ProcessOptions) { p.OutputChannel = c }
}
func WithPlannerType(t PlannerType) ProcessOptionFunc { return func(p *ProcessOptions) { p.PlannerType = t } }
func WithProcessType(t ProcessType) ProcessOptionFunc { return func(p *ProcessOptions) { p.ProcessType = t } }
