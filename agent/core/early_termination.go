package core

// EarlyTerminationPolicy decides whether a still-running process should be
// shut down at the current tick boundary. Each policy answers a yes/no with
// a human-readable reason; the runtime ORs them together via CompositePolicy.
type EarlyTerminationPolicy interface {
	ShouldTerminate(p Process) (terminate bool, reason string)
}

// MaxActionsPolicy cuts off a process after N action invocations (regardless
// of success). A guardrail against runaway re-planning or self-prompting
// loops in early prototypes.
type MaxActionsPolicy struct {
	Max int
}

// ShouldTerminate returns true when the process tree has executed at
// least Max actions in total. Reads the count from [Process.Usage]'s
// third return so child-process actions count toward the same budget.
// Processes that don't expose Usage() return false (the policy is
// effectively disabled).
func (p MaxActionsPolicy) ShouldTerminate(proc Process) (bool, string) {
	if p.Max <= 0 || proc == nil {
		return false, ""
	}

	usable, ok := proc.(interface {
		Usage() (cost float64, tokens int, actions int)
	})
	if !ok {
		return false, ""
	}

	if _, _, actions := usable.Usage(); actions < p.Max {
		return false, ""
	}
	return true, "max actions exceeded"
}

// BudgetPolicy fires when the running cost or token total breaches the
// configured ceilings. The Process is expected to expose a UsageSnapshot
// describing accumulated spend; processes without that snapshot effectively
// disable this policy.
type BudgetPolicy struct {
	Budget Budget
}

// ShouldTerminate enforces cost / token / action ceilings. A non-zero limit
// means "applies"; zero leaves that dimension unbounded.
func (p BudgetPolicy) ShouldTerminate(proc Process) (bool, string) {
	if proc == nil {
		return false, ""
	}

	usable, ok := proc.(interface {
		Usage() (cost float64, tokens int, actions int)
	})
	if !ok {
		return false, ""
	}

	cost, tokens, actions := usable.Usage()
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

// CompositePolicy is OR-of-policies: the first hit wins. This is the only
// safe combinator — AND would mean "terminate only if EVERY signal is
// firing" which is rarely what callers want.
type CompositePolicy struct {
	Policies []EarlyTerminationPolicy
}

// ShouldTerminate checks each child in order; the first to vote yes wins.
func (c CompositePolicy) ShouldTerminate(p Process) (bool, string) {
	for _, policy := range c.Policies {
		if stop, reason := policy.ShouldTerminate(p); stop {
			return true, reason
		}
	}
	return false, ""
}
