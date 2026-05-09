package plan

import (
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// PlanningSystem is the bag of capabilities passed to a planner. It mirrors
// embabel's AgentScope — but is detached from agent identity (a planner can
// reason over any subset).
type PlanningSystem struct {
	Actions    []core.Action
	Goals      []*core.Goal
	Conditions []core.Condition

	// knownConditions is the lazily-computed condition-key cache.
	// Initialised by the constructor via [sync.OnceValue]; subsequent
	// [PlanningSystem.KnownConditions] calls are a single function
	// call.
	knownConditions func() map[string]struct{}
}

// NewPlanningSystem constructs a system from explicit slices. Pass nil for
// any unused dimension; the planner tolerates empty inputs and returns nil
// plans gracefully.
func NewPlanningSystem(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *PlanningSystem {
	s := &PlanningSystem{Actions: actions, Goals: goals, Conditions: conditions}
	s.knownConditions = sync.OnceValue(func() map[string]struct{} {
		return core.KnownConditions(s.Actions, s.Goals, s.Conditions)
	})
	return s
}

// FromAgent builds a planning system out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func FromAgent(a *core.Agent) *PlanningSystem {
	if a == nil {
		return NewPlanningSystem(nil, nil, nil)
	}
	return NewPlanningSystem(a.Actions, a.Goals, a.Conditions)
}

// KnownConditions enumerates all condition keys reachable via the system —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (s *PlanningSystem) KnownConditions() map[string]struct{} {
	return s.knownConditions()
}
