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

// Valid reports whether s is a framework-defined action outcome.
func (s ActionStatus) Valid() bool {
	return s >= ActionSucceeded && s <= ActionPaused
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

// String renders the durable snake_case form a snapshot carries, the one
// [parseProcessStatus] reads back. These are stored values, not display text:
// renaming one silently orphans every snapshot already written with it.
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

func (s ProcessStatus) snapshotStable() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusStuck, StatusWaiting,
		StatusPaused, StatusTerminated, StatusKilled:
		return true
	default:
		return false
	}
}

// ReplanRequest tells the runtime that an action invalidated the current plan.
// The action must commit any discovered state through its ProcessContext before
// returning the request. The runtime excludes that action for one tick and
// plans again.
type ReplanRequest struct {
	Reason string
}

func (r *ReplanRequest) Error() string {
	if r == nil || r.Reason == "" {
		return "replan requested"
	}
	return "replan requested: " + r.Reason
}
