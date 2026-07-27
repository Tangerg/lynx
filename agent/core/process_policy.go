package core

import (
	"context"
	"strconv"
)

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

// Valid reports whether d is a framework-defined stuck decision.
func (d StuckDecision) Valid() bool {
	return d >= StuckStop && d <= StuckReplan
}

func (d StuckDecision) String() string {
	switch d {
	case StuckStop:
		return "stop"
	case StuckReplan:
		return "replan"
	default:
		return "StuckDecision(" + strconv.FormatUint(uint64(d), 10) + ")"
	}
}

// StuckResult carries the verdict plus a human-readable reason.
type StuckResult struct {
	Decision StuckDecision
	Reason   string
}

// StopPolicy decides whether a running process should terminate at the current
// tick boundary. A policy panic fails the process rather than escaping the
// runtime or being interpreted as a termination decision. Valid at engine and
// process scope. Tree budgets are runtime admission controls, not extensions.
type StopPolicy interface {
	Extension

	Check(process ProcessView) (stop bool, reason string)
}
