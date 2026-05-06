package plan

import (
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
)

// PlanningSystem is the bag of capabilities passed to a planner. It
// mirrors embabel's AgentScope — but is detached from agent identity
// (a planner can reason over any subset).
//
// Fields are private; readers go through Actions() / Goals() /
// Conditions() accessors. PlanningSystem is constructed by
// [NewPlanningSystem] or [FromAgent] and is treated as immutable
// thereafter — the cached KnownConditions() relies on this invariant.
type PlanningSystem struct {
	actions    []core.Action
	goals      []*core.Goal
	conditions []core.Condition

	knownConditions     atomic.Pointer[map[string]struct{}]
	knownConditionsOnce sync.Once
}

// NewPlanningSystem constructs a system from explicit slices. Pass nil for
// any unused dimension; the planner tolerates empty inputs and returns nil
// plans gracefully.
func NewPlanningSystem(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *PlanningSystem {
	return &PlanningSystem{actions: actions, goals: goals, conditions: conditions}
}

// FromAgent builds a planning system out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func FromAgent(a *core.Agent) *PlanningSystem {
	if a == nil {
		return &PlanningSystem{}
	}
	return &PlanningSystem{
		actions:    a.Actions(),
		goals:      a.Goals(),
		conditions: a.Conditions(),
	}
}

// Actions / Goals / Conditions are read-only views of the underlying
// slices. Slice contents alias the system's internal state — callers
// MUST NOT mutate.
func (s *PlanningSystem) Actions() []core.Action       { return s.actions }
func (s *PlanningSystem) Goals() []*core.Goal          { return s.goals }
func (s *PlanningSystem) Conditions() []core.Condition { return s.conditions }

// KnownConditions enumerates all condition keys reachable via the system —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (s *PlanningSystem) KnownConditions() map[string]struct{} {
	if cached := s.knownConditions.Load(); cached != nil {
		return *cached
	}

	s.knownConditionsOnce.Do(func() {
		computed := core.KnownConditions(s.actions, s.goals, s.conditions)
		s.knownConditions.Store(&computed)
	})
	return *s.knownConditions.Load()
}
