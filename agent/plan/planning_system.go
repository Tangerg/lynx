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
	return &PlanningSystem{Actions: a.Actions(), Goals: a.Goals(), Conditions: a.Conditions()}
}

// KnownConditions enumerates all condition keys reachable via the system —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (s *PlanningSystem) KnownConditions() map[string]struct{} {
	if cached := s.knownConditions.Load(); cached != nil {
		return *cached
	}

	s.knownConditionsOnce.Do(func() {
		computed := core.KnownConditions(s.Actions, s.Goals, s.Conditions)
		s.knownConditions.Store(&computed)
	})
	return *s.knownConditions.Load()
}

