package core

import "time"

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
		return "unknown"
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
		return "unknown"
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
	PlannerGOAP    PlannerType = iota // A* GOAP — default; strong guarantees on action sequencing.
	PlannerUtility                    // Utility scoring — open-ended exploration; not yet implemented.
)

// ProcessType chooses sequential vs. parallel action execution per tick.
type ProcessType int8

const (
	ProcessSimple     ProcessType = iota // One action per tick.
	ProcessConcurrent                    // Every applicable action of the plan in parallel.
)

// hasRunKey is the conventional condition key recording that a non-rerunnable
// action has executed at least once. The runtime sets it after each successful
// run; the planner consumes it as a precondition guard.
func HasRunKey(actionName string) string {
	return "hasRun_" + actionName
}

// nowFunc is overridable in tests so deterministic timestamps don't leak into
// HashKey or event payloads.
var nowFunc = time.Now

// Now returns the configured time source — production code should call this
// instead of time.Now so tests stay deterministic.
func Now() time.Time { return nowFunc() }

// SetNowFunc replaces the time source. Tests use this; production should not.
func SetNowFunc(fn func() time.Time) { nowFunc = fn }
