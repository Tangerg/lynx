package core

import "context"

// StuckHandler is invoked when the planner returns no plan. The default is
// "give up and transition to StatusStuck"; agents that want graceful
// degradation can supply a handler that mutates the blackboard (relax a
// constraint, drop a goal, ...) and request a re-plan.
type StuckHandler interface {
	HandleStuck(ctx context.Context, p Process) StuckHandlingResult
}

// StuckHandlingCode is the handler's verdict.
type StuckHandlingCode int8

const (
	StuckReplan       StuckHandlingCode = iota // Re-plan after the handler's mutations.
	StuckNoResolution                          // Surrender; runtime sets StatusStuck.
)

// StuckHandlingResult carries the verdict plus a human-readable reason.
type StuckHandlingResult struct {
	Code   StuckHandlingCode
	Reason string
}

// StuckHandlerFunc adapts a plain function into the StuckHandler interface.
type StuckHandlerFunc func(ctx context.Context, p Process) StuckHandlingResult

func (f StuckHandlerFunc) HandleStuck(ctx context.Context, p Process) StuckHandlingResult {
	return f(ctx, p)
}
