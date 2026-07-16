package event

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
)

// ProcessCreated fires when a new Process is registered on the
// engine — captures the initial bindings the caller seeded.
type ProcessCreated struct {
	Header
	Bindings map[string]any `json:"bindings,omitempty"`
}

func (ProcessCreated) Kind() string { return "process_created" }

// ProcessCompleted fires when the process reaches its goal
// successfully.
type ProcessCompleted struct {
	Header
	Goal   *core.Goal `json:"-"`
	Result any        `json:"-"`
}

func (ProcessCompleted) Kind() string { return "process_completed" }

// ProcessFailed fires when the process terminates with an error.
type ProcessFailed struct {
	Header
	Err error `json:"-"`
}

func (ProcessFailed) Kind() string { return "process_failed" }

// ProcessStuck fires when the planner returns no plan and no
// StuckPolicy resolves it.
type ProcessStuck struct {
	Header
	State core.WorldState `json:"-"`
}

func (ProcessStuck) Kind() string { return "process_stuck" }

// ProcessWaiting fires when a process parks durable continuation state.
type ProcessWaiting struct {
	Header
	Suspension *interaction.Suspension `json:"suspension"`
}

func (ProcessWaiting) Kind() string { return "process_waiting" }

// ProcessSnapshotFailed reports that automatic persistence did not commit.
// Report-only policy means the process may continue but is explicitly
// degraded rather than being represented as durable.
type ProcessSnapshotFailed struct {
	Header
	Policy string `json:"policy"`
	Err    error  `json:"-"`
}

func (ProcessSnapshotFailed) Kind() string { return "process_snapshot_failed" }

// ProcessKilled fires from Engine.Kill or when ctx is
// canceled mid-run.
type ProcessKilled struct {
	Header
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilled) Kind() string { return "process_killed" }

// ProcessTerminated fires when a StopPolicy or a
// queued [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminated struct {
	Header
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminated) Kind() string { return "process_terminated" }
