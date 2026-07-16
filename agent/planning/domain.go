package planning

import (
	"maps"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// Domain is an immutable capability set passed to a planner, detached from
// agent identity so a planner can reason over any subset.
type Domain struct {
	actions         []core.Action
	goals           []*core.Goal
	conditions      []core.Condition
	knownConditions map[string]struct{}
}

// NewDomain constructs a domain from explicit slices. Pass nil for
// any unused dimension; the planner tolerates empty inputs and returns nil
// plans gracefully.
func NewDomain(actions []core.Action, goals []*core.Goal, conditions []core.Condition) *Domain {
	domain := &Domain{
		actions:    slices.Clone(actions),
		goals:      slices.Clone(goals),
		conditions: slices.Clone(conditions),
	}
	domain.knownConditions = domain.computeKnownConditions()
	return domain
}

// Actions returns a snapshot of the available actions.
func (d *Domain) Actions() []core.Action {
	if d == nil {
		return nil
	}
	return slices.Clone(d.actions)
}

// Goals returns a snapshot of the candidate goals.
func (d *Domain) Goals() []*core.Goal {
	if d == nil {
		return nil
	}
	return slices.Clone(d.goals)
}

// Conditions returns a snapshot of the named condition implementations.
func (d *Domain) Conditions() []core.Condition {
	if d == nil {
		return nil
	}
	return slices.Clone(d.conditions)
}

// DomainForAgent builds a planning domain out of an agent's capability set —
// convenience for the runtime which wires planner ↔ agent.
func DomainForAgent(agent *core.Agent) *Domain {
	if agent == nil {
		return NewDomain(nil, nil, nil)
	}
	return NewDomain(agent.Actions(), agent.Goals(), agent.Conditions())
}

// DomainForAgents unions the capability sets of multiple agents into a single
// planning domain — joint planning across agent boundaries. The resulting domain carries the concatenation of every
// agent's actions, goals, and conditions; the planner reasons over the
// whole union and may pick a path that crosses agent boundaries.
//
// Name uniqueness across the input agents is the caller's
// responsibility — the planner does not deduplicate. Nil entries are
// skipped so callers can pass partially-populated slices without
// guarding.
func DomainForAgents(agents []*core.Agent) *Domain {
	var (
		actions    []core.Action
		goals      []*core.Goal
		conditions []core.Condition
	)
	for _, agent := range agents {
		if agent == nil {
			continue
		}
		actions = append(actions, agent.Actions()...)
		goals = append(goals, agent.Goals()...)
		conditions = append(conditions, agent.Conditions()...)
	}
	return NewDomain(actions, goals, conditions)
}

// KnownConditions enumerates all condition keys reachable via the domain —
// the world-state determiner uses it to know what to evaluate. Result is
// cached after the first call.
func (d *Domain) KnownConditions() map[string]struct{} {
	if d == nil {
		return nil
	}
	return maps.Clone(d.knownConditions)
}

func (d *Domain) computeKnownConditions() map[string]struct{} {
	conditions := map[string]struct{}{}
	for _, action := range d.actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		for key := range metadata.Preconditions {
			conditions[key] = struct{}{}
		}
		for key := range metadata.Effects {
			conditions[key] = struct{}{}
		}
	}
	for _, goal := range d.goals {
		if goal == nil {
			continue
		}
		for key := range goal.Preconditions() {
			conditions[key] = struct{}{}
		}
	}
	for _, condition := range d.conditions {
		if condition != nil {
			conditions[condition.Name()] = struct{}{}
		}
	}
	return conditions
}
