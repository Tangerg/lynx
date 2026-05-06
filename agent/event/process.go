package event

import "github.com/Tangerg/lynx/agent/core"

// ProcessCreatedEvent fires when a new AgentProcess is registered on the
// platform — captures the initial bindings the caller seeded.
type ProcessCreatedEvent struct {
	BaseEvent
	Bindings map[string]any `json:"bindings,omitempty"`
}

func (ProcessCreatedEvent) EventName() string { return "process_created" }

func (e ProcessCreatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"bindings": e.Bindings})
}

// ProcessCompletedEvent fires when the process reaches its goal
// successfully.
type ProcessCompletedEvent struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (ProcessCompletedEvent) EventName() string { return "process_completed" }

func (e ProcessCompletedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}

// ProcessFailedEvent fires when the process terminates with an error.
type ProcessFailedEvent struct {
	BaseEvent
	Err error `json:"-"`
}

func (ProcessFailedEvent) EventName() string { return "process_failed" }

func (e ProcessFailedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"error": errString(e.Err)})
}

// ProcessStuckEvent fires when the planner returns no plan and no
// StuckHandler resolves it.
type ProcessStuckEvent struct {
	BaseEvent
	LastWorld core.WorldState `json:"-"`
}

func (ProcessStuckEvent) EventName() string { return "process_stuck" }

func (e ProcessStuckEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.LastWorld)})
}

// ProcessWaitingEvent fires when a typed action calls AwaitInput and the
// process suspends pending external input.
type ProcessWaitingEvent struct {
	BaseEvent
	Awaitable core.Awaitable `json:"-"`
}

func (ProcessWaitingEvent) EventName() string { return "process_waiting" }

func (e ProcessWaitingEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"awaitable": summarizeAwaitable(e.Awaitable)})
}

// ProcessPausedEvent is reserved for integration code that wants to flag
// non-suspending pauses (rate limits, cooldowns). Framework doesn't emit
// it directly today.
type ProcessPausedEvent struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessPausedEvent) EventName() string { return "process_paused" }

func (e ProcessPausedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

// ProcessKilledEvent fires from Platform.KillProcess or when ctx is
// cancelled mid-run.
type ProcessKilledEvent struct {
	BaseEvent
	Reason string `json:"reason,omitempty"`
}

func (ProcessKilledEvent) EventName() string { return "process_killed" }

func (e ProcessKilledEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason})
}

// ProcessTerminatedEvent fires when an EarlyTerminationPolicy or a
// queued [core.TerminationScopeAgent] signal stops the process.
type ProcessTerminatedEvent struct {
	BaseEvent
	Reason string                `json:"reason,omitempty"`
	Scope  core.TerminationScope `json:"-"`
}

func (ProcessTerminatedEvent) EventName() string { return "process_terminated" }

func (e ProcessTerminatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"reason": e.Reason, "scope": e.Scope.String()})
}
