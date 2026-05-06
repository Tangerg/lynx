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

// StuckHandlerFunc adapts a plain function into the StuckHandler interface.
type StuckHandlerFunc func(ctx context.Context, p Process) StuckResult

func (f StuckHandlerFunc) HandleStuck(ctx context.Context, p Process) StuckResult {
	return f(ctx, p)
}
