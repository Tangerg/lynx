package core

import "context"

// StuckHandler is invoked when the planner returns no plan. The default is
// "give up and transition to StatusStuck"; agents that want graceful
// degradation can supply a handler that mutates the blackboard (relax a
// constraint, drop a goal, ...) and request a re-plan.
type StuckHandler interface {
	HandleStuck(ctx context.Context, p Process) StuckResult
}

// StuckHandlingCode is the handler's verdict.
type StuckHandlingCode int8

const (
	StuckReplan       StuckHandlingCode = iota // Re-plan after the handler's mutations.
	StuckNoResolution                          // Surrender; runtime sets StatusStuck.
)

// StuckResult carries the verdict plus a human-readable reason.
type StuckResult struct {
	Code   StuckHandlingCode
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
