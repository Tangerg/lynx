package event

import "github.com/Tangerg/lynx/agent/core"

// ProcessCreated fires when a new AgentProcess is registered on the
// platform — captures the initial bindings the caller seeded.
type ProcessCreated struct {
	BaseEvent
	Bindings map[string]any `json:"bindings,omitempty"`
}

func (ProcessCreated) EventName() string { return "process_created" }

// ProcessCompleted fires when the process reaches its goal
// successfully.
type ProcessCompleted struct {
	BaseEvent
	Goal   *core.Goal `json:"-"`
	Result any        `json:"-"`
}

func (ProcessCompleted) EventName() string { return "process_completed" }

// ProcessFailed fires when the process terminates with an error.
type ProcessFailed struct {
	BaseEvent
	Err error `json:"-"`
}

func (ProcessFailed) EventName() string { return "process_failed" }

// ProcessStuck fires when the planner returns no plan and no
// StuckPolicy resolves it.
type ProcessStuck struct {
	BaseEvent
	LastWorld core.WorldState `json:"-"`
}

func (ProcessStuck) EventName() string { return "process_stuck" }

// ProcessWaiting fires when a typed action calls AwaitInput and the
// process suspends pending external input.
type ProcessWaiting struct {
	BaseEvent
	Awaitable core.Awaitable `json:"-"`
}

func (ProcessWaiting) EventName() string { return "process_waiting" }

// ProcessKilled fires from Platform.KillProcess or when ctx is
// canceled mid-run.
type ProcessKilled struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilled) EventName() string { return "process_killed" }

// ProcessTerminated fires when an EarlyTerminationPolicy or a
// queued [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminated struct {
	BaseEvent
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminated) EventName() string { return "process_terminated" }
