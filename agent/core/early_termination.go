package core

// EarlyTerminationPolicy decides whether a still-running process
// should be shut down at the current tick boundary. Each policy
// answers a yes/no with a human-readable reason.
type EarlyTerminationPolicy interface {
	ShouldTerminate(p Process) (terminate bool, reason string)
}

// BudgetPolicy fires when running cost / token / action total
// breaches the configured ceilings. Reads [Process.Usage] for the
// subtree-aggregated totals.
type BudgetPolicy struct {
	Budget Budget
}

// ShouldTerminate enforces cost / token / action ceilings. A
// non-zero limit means "applies"; zero leaves that dimension
// unbounded.
func (p BudgetPolicy) ShouldTerminate(proc Process) (bool, string) {
	if proc == nil {
		return false, ""
	}
	cost, tokens, actions := proc.Usage()
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
