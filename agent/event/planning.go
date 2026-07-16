package event

import (
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/planning"
)

// PlanningStarted reports the world state the planner is about to consume.
type PlanningStarted struct {
	Header
	State core.WorldState `json:"-"`
}

func (PlanningStarted) Kind() string { return "planning_started" }

// PlanCreated fires when the planner returns a non-nil plan.
type PlanCreated struct {
	Header
	Plan *planning.Plan `json:"-"`
}

func (PlanCreated) Kind() string { return "plan_created" }

// ReplanRequested fires when an action returns a [core.ReplanRequest]
// or a [core.TerminationScopeAction] signal is queued — both ask the
// runtime to re-plan on the next tick.
type ReplanRequested struct {
	Header
	ActionName string `json:"action,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

func (ReplanRequested) Kind() string { return "replan_requested" }
