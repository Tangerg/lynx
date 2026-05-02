package plan

import (
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
)

// PlanningSystem is the bag of capabilities passed to a planner. It mirrors
// embabel's AgentScope — but is detached from agent identity (a planner can
// reason over any subset).
type PlanningSystem struct {
	Actions    []core.Action
	Goals      []*core.Goal
	Conditions []core.Condition

	knownConditions     atomic.Pointer[map[string]struct{}]
	knownConditionsOnce sync.Once
}

// NewPlanningSystem constructs a system from explicit slices. Pass nil for
// any unused dimension; the planner tolerates empty inputs and returns nil
// plans gracefully.
func NewPlanningSystem(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *PlanningSystem {
	return &PlanningSystem{Actions: actions, Goals: goals, Conditions: conditions}
}

// FromAgent builds a planning system out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func FromAgent(a *core.Agent) *PlanningSystem {
	if a == nil {
		return &PlanningSystem{}
	}
	return &PlanningSystem{Actions: a.Actions, Goals: a.Goals, Conditions: a.Conditions}
}

// KnownConditions enumerates all condition keys reachable via the system —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (s *PlanningSystem) KnownConditions() map[string]struct{} {
	if cached := s.knownConditions.Load(); cached != nil {
		return *cached
	}

	s.knownConditionsOnce.Do(func() {
		computed := s.computeKnownConditions()
		s.knownConditions.Store(&computed)
	})
	return *s.knownConditions.Load()
}

func (s *PlanningSystem) computeKnownConditions() map[string]struct{} {
	out := map[string]struct{}{}

	for _, action := range s.Actions {
		meta := action.Metadata()
		for key := range meta.Preconditions {
			out[key] = struct{}{}
		}
		for key := range meta.Effects {
			out[key] = struct{}{}
		}
	}

	for _, goal := range s.Goals {
		for key := range goal.Preconditions() {
			out[key] = struct{}{}
		}
	}

	for _, cond := range s.Conditions {
		out[cond.Name()] = struct{}{}
	}
	return out
}

// FindAction is the small lookup helper. Linear scan; the action list is
// always small in practice.
func (s *PlanningSystem) FindAction(name string) (core.Action, bool) {
	for _, action := range s.Actions {
		if action.Metadata().Name == name {
			return action, true
		}
	}
	return nil, false
}

// FindGoal mirrors FindAction.
func (s *PlanningSystem) FindGoal(name string) (*core.Goal, bool) {
	for _, goal := range s.Goals {
		if goal.Name == name {
			return goal, true
		}
	}
	return nil, false
}
