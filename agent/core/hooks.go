package core

import "context"

// StuckPolicy is invoked when the planner returns no plan. The
// default behaviour is "give up and transition to StatusStuck";
// agents that want graceful degradation can supply a policy that
// mutates the blackboard (relax a constraint, drop a goal, …) and
// requests a re-plan.
//
// Parallels [EarlyTerminationPolicy] in shape: each policy returns
// a verdict the runtime acts on.
type StuckPolicy interface {
	// Recover decides what to do when planning has stalled. Return
	// [StuckResult] with Code = StuckReplan to retry after any
	// mutations, or StuckNoResolution to surrender.
	Recover(ctx context.Context, p Process) StuckResult
}

// StuckCode is the verdict a [StuckPolicy] returns.
type StuckCode int8

const (
	StuckReplan       StuckCode = iota // Re-plan after the policy's mutations.
	StuckNoResolution                  // Surrender; runtime sets StatusStuck.
)

// StuckResult carries the verdict plus a human-readable reason.
type StuckResult struct {
	Code   StuckCode
	Reason string
}

// ReplanRequest is the Go-flavored replacement for embabel's
// ReplanRequestedException. An action that decides "what I just learned
// invalidates the current plan" returns one as an error; the runtime
// extracts it via [errors.AsType], blacklists the offending action for
// one tick, and reformulates the plan.
type ReplanRequest struct {
	Reason string

	// Update runs before re-planning so the action can stage the change
	// (e.g. "set route=alternate") that motivated the re-plan.
	Update func(Blackboard)
}

func (r *ReplanRequest) Error() string {
	if r == nil || r.Reason == "" {
		return "replan requested"
	}
	return "replan requested: " + r.Reason
}
