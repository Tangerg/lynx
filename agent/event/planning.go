package event

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// ReadyToPlan fires at the start of every tick after the
// world-state determiner has run — gives listeners a snapshot of what
// the planner is about to see.
type ReadyToPlan struct {
	BaseEvent
	World core.WorldState `json:"-"`
}

func (ReadyToPlan) EventName() string { return "ready_to_plan" }

func (e ReadyToPlan) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.World)})
}

// PlanFormulated fires when the planner returns a non-nil plan.
type PlanFormulated struct {
	BaseEvent
	Plan *plan.Plan `json:"-"`
}

func (PlanFormulated) EventName() string { return "plan_formulated" }

func (e PlanFormulated) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"plan": summarizePlan(e.Plan)})
}

// ReplanRequested fires when an action returns a [core.ReplanRequest]
// or a [core.TerminationScopeAction] signal is queued — both ask the
// runtime to re-plan on the next tick.
type ReplanRequested struct {
	BaseEvent
	Action string `json:"action,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (ReplanRequested) EventName() string { return "replan_requested" }

func (e ReplanRequested) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": e.Action, "reason": e.Reason})
}
