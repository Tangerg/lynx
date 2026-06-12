package planning

import (
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// System is the bag of capabilities passed to a planner — detached from
// agent identity (a planner can reason over any subset).
type System struct {
	Actions    []core.Action
	Goals      []*core.Goal
	Conditions []core.Condition

	// knownConditions is the lazily-computed condition-key cache.
	// Initialized by the constructor via [sync.OnceValue]; subsequent
	// [System.KnownConditions] calls are a single function
	// call.
	knownConditions func() map[string]struct{}
}

// NewSystem constructs a system from explicit slices. Pass nil for
// any unused dimension; the planner tolerates empty inputs and returns nil
// plans gracefully.
func NewSystem(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *System {
	s := &System{Actions: actions, Goals: goals, Conditions: conditions}
	s.knownConditions = sync.OnceValue(func() map[string]struct{} {
		return core.KnownConditions(s.Actions, s.Goals, s.Conditions)
	})
	return s
}

// FromAgent builds a planning system out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func FromAgent(a *core.Agent) *System {
	if a == nil {
		return NewSystem(nil, nil, nil)
	}
	return NewSystem(a.Actions, a.Goals, a.Conditions)
}

// FromAgents unions the capability sets of multiple agents into a single
// planning system — joint planning across agent boundaries. The resulting system carries the concatenation of every
// agent's actions, goals, and conditions; the planner reasons over the
// whole union and may pick a path that crosses agent boundaries.
//
// Name uniqueness across the input agents is the caller's
// responsibility — the planner does not deduplicate. Nil entries are
// skipped so callers can pass partially-populated slices without
// guarding.
func FromAgents(agents []*core.Agent) *System {
	var (
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	)
	for _, a := range agents {
		if a == nil {
			continue
		}
		actions = append(actions, a.Actions...)
		goals = append(goals, a.Goals...)
		conditions = append(conditions, a.Conditions...)
	}
	return NewSystem(actions, goals, conditions)
}

// KnownConditions enumerates all condition keys reachable via the system —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (s *System) KnownConditions() map[string]struct{} {
	return s.knownConditions()
}
