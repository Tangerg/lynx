package event

import (
	"time"

	"github.com/Tangerg/lynx/agent/core"
)

// ActionExecutionStartEvent fires before an action is invoked (per
// retry attempt the runtime publishes only on the outer call, not per
// retry).
type ActionExecutionStartEvent struct {
	BaseEvent
	Action    core.Action `json:"-"`
	StartedAt time.Time   `json:"-"`
}

func (ActionExecutionStartEvent) EventName() string { return "action_execution_start" }

func (e ActionExecutionStartEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": actionName(e.Action), "started_at": e.StartedAt})
}

// ActionExecutionResultEvent fires after an action's retry loop
// terminates — Status carries the final outcome, Err the last error
// (may be nil on success).
type ActionExecutionResultEvent struct {
	BaseEvent
	Action   core.Action       `json:"-"`
	Status   core.ActionStatus `json:"-"`
	Duration time.Duration     `json:"-"`
	Err      error             `json:"-"`
}

func (ActionExecutionResultEvent) EventName() string { return "action_execution_result" }

func (e ActionExecutionResultEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{
		"action":      actionName(e.Action),
		"status":      e.Status.String(),
		"duration_ns": e.Duration.Nanoseconds(),
		"error":       errString(e.Err),
	})
}

// GoalAchievedEvent fires when the planner returns an empty plan for a
// non-nil goal (i.e. preconditions are already satisfied).
type GoalAchievedEvent struct {
	BaseEvent
	Goal *core.Goal `json:"-"`
}

func (GoalAchievedEvent) EventName() string { return "goal_achieved" }

func (e GoalAchievedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"goal": summarizeGoal(e.Goal)})
}
