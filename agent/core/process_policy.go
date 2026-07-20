package core

import "context"

// StuckPolicy is invoked when the planner returns no plan. The default is to
// transition to StatusStuck; a policy may update the blackboard and request a
// new planning pass.
type StuckPolicy interface {
	Recover(ctx context.Context, process ProcessView, blackboard BlackboardWriter) StuckResult
}

// StuckDecision is the verdict returned by a StuckPolicy.
type StuckDecision uint8

const (
	// StuckStop leaves the process in StatusStuck. It is the safe zero value.
	StuckStop StuckDecision = iota
	// StuckReplan asks the runtime to plan again after policy mutations.
	StuckReplan
)

// StuckResult carries the verdict plus a human-readable reason.
type StuckResult struct {
	Decision StuckDecision
	Reason   string
}

// StopPolicy decides whether a running process should terminate at the current
// tick boundary. Registered policies are checked alongside the implicit
// budget-derived policy.
type StopPolicy interface {
	Extension

	Check(process ProcessView) (stop bool, reason string)
}

// BudgetPolicy terminates a process whose subtree usage reaches a configured
// cost, token, or action ceiling.
type BudgetPolicy struct {
	Budget Budget
}

// Name is the extension identifier for the built-in budget policy.
func (p BudgetPolicy) Name() string { return "budget-policy" }

// Check enforces non-zero cost, token, and action ceilings.
func (p BudgetPolicy) Check(process ProcessView) (bool, string) {
	if process == nil {
		return false, ""
	}
	cost, tokens, actions := process.Usage()
	switch {
	case p.Budget.CostLimit > 0 && cost >= p.Budget.CostLimit:
		return true, "cost budget exceeded"
	case p.Budget.TokenLimit > 0 && tokens >= p.Budget.TokenLimit:
		return true, "token budget exceeded"
	case p.Budget.ActionLimit > 0 && actions >= p.Budget.ActionLimit:
		return true, "action budget exceeded"
	}
	return false, ""
}
