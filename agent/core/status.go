package core

import (
	"fmt"
)

// ActionStatus is the outcome of a single Action.Execute call.
type ActionStatus int8

const (
	ActionSucceeded ActionStatus = iota
	ActionFailed
	ActionWaiting // Suspension parked — process should wait for external input.
	ActionPaused  // Action voluntarily yielded; runtime may resume later.
)

func (s ActionStatus) String() string {
	switch s {
	case ActionSucceeded:
		return "succeeded"
	case ActionFailed:
		return "failed"
	case ActionWaiting:
		return "waiting"
	case ActionPaused:
		return "paused"
	default:
		return fmt.Sprintf("unknown_action_status(%d)", s)
	}
}

// ProcessStatus tracks the lifecycle of a single Process.
type ProcessStatus int8

const (
	StatusNotStarted ProcessStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusStuck
	StatusWaiting
	StatusPaused
	StatusTerminated
	StatusKilled
)

func (s ProcessStatus) String() string {
	switch s {
	case StatusNotStarted:
		return "not_started"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusStuck:
		return "stuck"
	case StatusWaiting:
		return "waiting"
	case StatusPaused:
		return "paused"
	case StatusTerminated:
		return "terminated"
	case StatusKilled:
		return "killed"
	default:
		return fmt.Sprintf("unknown_process_status(%d)", s)
	}
}

// IsTerminal reports whether a process in this state has stopped advancing on
// its own — runtime loops use this to decide when to break out of tick.
func (s ProcessStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusStuck, StatusTerminated, StatusKilled:
		return true
	default:
		return false
	}
}

func (s ProcessStatus) valid() bool {
	switch s {
	case StatusNotStarted, StatusRunning, StatusCompleted, StatusFailed, StatusStuck, StatusWaiting, StatusPaused, StatusTerminated, StatusKilled:
		return true
	default:
		return false
	}
}

// ReplanRequest tells the runtime that an action invalidated the current plan.
// The runtime excludes that action for one tick, applies Update, and plans
// again.
type ReplanRequest struct {
	Reason string

	// Update stages state discovered by the action before re-planning.
	Update func(Blackboard)
}

func (r *ReplanRequest) Error() string {
	if r == nil || r.Reason == "" {
		return "replan requested"
	}
	return "replan requested: " + r.Reason
}

// TerminationScope identifies the boundary affected by a termination request.
type TerminationScope int

const (
	// TerminationScopeAgent stops the entire process.
	TerminationScopeAgent TerminationScope = iota

	// TerminationScopeAction stops the current action and replans.
	TerminationScopeAction
)

func (s TerminationScope) String() string {
	switch s {
	case TerminationScopeAgent:
		return "agent"
	case TerminationScopeAction:
		return "action"
	default:
		return "unknown"
	}
}

// TerminationSignal is a pending structured termination request.
type TerminationSignal struct {
	Scope  TerminationScope
	Reason string
}
