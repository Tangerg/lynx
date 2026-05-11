package core

// EarlyTerminationPolicy decides whether a still-running process
// should be shut down at the current tick boundary. Each policy
// answers a yes/no with a human-readable reason.
//
// EarlyTerminationPolicy is also a platform [Extension]: register
// one or more on the platform (or per-process via
// [ProcessOptions.Extensions]) and the runtime asks every registered
// policy each tick — any policy returning true terminates the
// process. The framework always checks the Budget-derived policy
// implicitly, so a zero-extensions setup still enforces
// [ProcessOptions.Budget].
type EarlyTerminationPolicy interface {
	Extension

	ShouldTerminate(p Process) (terminate bool, reason string)
}

// BudgetPolicy fires when running cost / token / action total
// breaches the configured ceilings. Reads [Process.Usage] for the
// subtree-aggregated totals. The runtime checks an instance of this
// policy implicitly using [ProcessOptions.Budget]; explicit
// registration is only needed when the user wants a second budget
// (e.g. a stricter per-step budget alongside the per-process one).
type BudgetPolicy struct {
	Budget Budget
}

// Name is the extension identifier for the built-in budget policy.
// User-supplied [BudgetPolicy] values registered as extensions
// should use a distinct name to avoid colliding with the framework
// default.
func (p BudgetPolicy) Name() string { return "budget-policy" }

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
