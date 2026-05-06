package event

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/plan"
)

// ReadyToPlanEvent fires at the start of every tick after the
// world-state determiner has run — gives listeners a snapshot of what
// the planner is about to see.
type ReadyToPlanEvent struct {
	BaseEvent
	World core.WorldState `json:"-"`
}

func (ReadyToPlanEvent) EventName() string { return "ready_to_plan" }

func (e ReadyToPlanEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"world": snapshotWorld(e.World)})
}

// PlanFormulatedEvent fires when the planner returns a non-nil plan.
type PlanFormulatedEvent struct {
	BaseEvent
	Plan *plan.Plan `json:"-"`
}

func (PlanFormulatedEvent) EventName() string { return "plan_formulated" }

func (e PlanFormulatedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"plan": summarizePlan(e.Plan)})
}

// ReplanRequestedEvent fires when an action returns a [core.ReplanRequest]
// or a [core.TerminationScopeAction] signal is queued — both ask the
// runtime to re-plan on the next tick.
type ReplanRequestedEvent struct {
	BaseEvent
	Action string `json:"action,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (ReplanRequestedEvent) EventName() string { return "replan_requested" }

func (e ReplanRequestedEvent) MarshalJSON() ([]byte, error) {
	return emit(e, map[string]any{"action": e.Action, "reason": e.Reason})
}
