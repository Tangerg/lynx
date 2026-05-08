package core

import (
	"fmt"
	"time"
)

// ActionStatus is the outcome of a single Action.Execute call.
type ActionStatus int8

const (
	ActionSucceeded ActionStatus = iota
	ActionFailed
	ActionWaiting // Awaitable returned — process should pause for external input.
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

// AgentProcessStatus tracks the lifecycle of a single AgentProcess.
type AgentProcessStatus int8

const (
	StatusNotStarted AgentProcessStatus = iota
	StatusRunning
	StatusCompleted
	StatusFailed
	StatusStuck
	StatusWaiting
	StatusPaused
	StatusTerminated
	StatusKilled
)

func (s AgentProcessStatus) String() string {
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
func (s AgentProcessStatus) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusStuck, StatusTerminated, StatusKilled:
		return true
	default:
		return false
	}
}

// PlannerType selects which planner the platform builds for a process.
type PlannerType int8

const (
	// PlannerGOAP — A* GOAP. Default; strong guarantees on action
	// sequencing; finds an optimal plan when one exists.
	PlannerGOAP PlannerType = iota

	// PlannerHTN — hierarchical task network. The runtime's default
	// PlannerFactory does not build one (HTN needs a user-supplied
	// task library); register a custom PlannerFactory extension that
	// returns *htn.Planner.
	PlannerHTN

	// PlannerReactive — greedy one-step utility scoring. Picks the
	// action whose effects close the most goal preconditions, ties
	// broken by lower cost. Suited to event-driven loops where the
	// world changes between ticks. Mirrors embabel's UtilityPlanner.
	PlannerReactive
)

func (t PlannerType) String() string {
	switch t {
	case PlannerGOAP:
		return "goap"
	case PlannerHTN:
		return "htn"
	case PlannerReactive:
		return "reactive"
	default:
		return fmt.Sprintf("unknown_planner_type(%d)", t)
	}
}

// ProcessType chooses sequential vs. parallel action execution per tick.
type ProcessType int8

const (
	ProcessSequential ProcessType = iota // One action per tick.
	ProcessConcurrent                    // Every applicable action of the plan in parallel.
)

func (t ProcessType) String() string {
	switch t {
	case ProcessSequential:
		return "sequential"
	case ProcessConcurrent:
		return "concurrent"
	default:
		return fmt.Sprintf("unknown_process_type(%d)", t)
	}
}

// Now is the framework's time source — production code uses this so a
// future override (e.g. for deterministic tests) lives in one place.
// Currently a thin wrapper over [time.Now].
func Now() time.Time { return time.Now() }
