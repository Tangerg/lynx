package event

import "github.com/Tangerg/lynx/agent/core"

// ProcessCreated fires when a new AgentProcess is registered on the
// platform — captures the initial bindings the caller seeded.
type ProcessCreated struct {
	BaseEvent
	Bindings map[string]any `json:"bindings,omitempty"`
}

func (ProcessCreated) EventName() string { return "process_created" }

func (e ProcessCreated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"bindings": e.Bindings})
}

// ProcessCompleted fires when the process reaches its goal
// successfully.
type ProcessCompleted struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (ProcessCompleted) EventName() string { return "process_completed" }

func (e ProcessCompleted) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}

// ProcessFailed fires when the process terminates with an error.
type ProcessFailed struct {
	BaseEvent
	Err error `json:"-"`
}

func (ProcessFailed) EventName() string { return "process_failed" }

func (e ProcessFailed) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"error": errString(e.Err)})
}

// ProcessStuck fires when the planner returns no plan and no
// StuckHandler resolves it.
type ProcessStuck struct {
	BaseEvent
	LastWorld core.WorldState `json:"-"`
}

func (ProcessStuck) EventName() string { return "process_stuck" }

func (e ProcessStuck) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.LastWorld)})
}

// ProcessWaiting fires when a typed action calls AwaitInput and the
// process suspends pending external input.
type ProcessWaiting struct {
	BaseEvent
	Awaitable core.Awaitable `json:"-"`
}

func (ProcessWaiting) EventName() string { return "process_waiting" }

func (e ProcessWaiting) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"awaitable": summarizeAwaitable(e.Awaitable)})
}

// ProcessKilled fires from Platform.KillProcess or when ctx is
// cancelled mid-run.
type ProcessKilled struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilled) EventName() string { return "process_killed" }

func (e ProcessKilled) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

// ProcessTerminated fires when an EarlyTerminationPolicy or a
// queued [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminated struct {
	BaseEvent
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminated) EventName() string { return "process_terminated" }

func (e ProcessTerminated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason, "scope": e.Scope.String()})
}
