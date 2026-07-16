package core

import "context"

// StuckPolicy is invoked when the planner returns no plan. The
// default behavior is "give up and transition to StatusStuck";
// agents that want graceful degradation can supply a policy that
// mutates the blackboard (relax a constraint, drop a goal, …) and
// requests a re-plan.
//
// Parallels [StopPolicy] in shape: each policy returns
// a verdict the runtime acts on.
type StuckPolicy interface {
	// Recover decides what to do when planning has stalled. Return
	// [StuckResult] with Decision = StuckReplan to retry after any
	// mutations. The zero result stops planning safely.
	Recover(ctx context.Context, process ProcessView, blackboard BlackboardWriter) StuckResult
}

// StuckDecision is the verdict a [StuckPolicy] returns.
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
