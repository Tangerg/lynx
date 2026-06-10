package planning

import (
	"slices"

	"github.com/Tangerg/lynx/agent/core"
)

// Plan is the planner's output: an ordered list of actions whose
// accumulated effects achieve Goal.Preconditions. An empty action list with
// a non-nil Goal means "the goal is already satisfied".
type Plan struct {
	Actions []core.Action
	Goal    *core.Goal
}

// IsComplete reports "we don't need to do any more work for this goal".
func (p *Plan) IsComplete() bool {
	return p == nil || len(p.Actions) == 0
}

// Cost is the sum of action costs; the planner uses it to rank competing
// plans. It samples each action's cost against the supplied world state so
// dynamic-cost actions get evaluated correctly. Actions with a nil Cost
// contribute nothing — the canonical construction path ([core.NewAction])
// fills in [core.Static](1.0).
func (p *Plan) Cost(worldState core.WorldState) float64 {
	if p == nil {
		return 0
	}

	total := 0.0
	for _, action := range p.Actions {
		if action == nil {
			continue
		}
		if fn := action.Metadata().Cost; fn != nil {
			total += fn(worldState)
		}
	}
	return total
}

// Value is the goal value; cached here so callers don't have to
// dereference (p.Goal nil-check + Value resolution) themselves. A nil
// Goal.Value contributes 0 — [core.GoalProducing] fills in
// [core.Static](1.0).
func (p *Plan) Value(worldState core.WorldState) float64 {
	if p == nil || p.Goal == nil || p.Goal.Value == nil {
		return 0
	}
	return p.Goal.Value(worldState)
}

// ActionsValue is the sum of the plan's action values, sampled against the
// supplied world state so dynamic-value actions get evaluated correctly.
// Actions with a nil Value contribute nothing — the canonical construction
// path ([core.NewAction]) fills in [core.Static](0), so this term is zero
// unless an action opts into a non-trivial value.
func (p *Plan) ActionsValue(worldState core.WorldState) float64 {
	if p == nil {
		return 0
	}

	total := 0.0
	for _, action := range p.Actions {
		if action == nil {
			continue
		}
		if fn := action.Metadata().Value; fn != nil {
			total += fn(worldState)
		}
	}
	return total
}

// NetValue ranks competing plans: goal value plus the accumulated value of
// the plan's actions, minus total plan cost. Mirrors embabel's Plan.netValue
// (goal.value + actionsValue − cost) — the actions-value term rewards plans
// whose constituent actions are independently valuable, not just the cheapest
// path to the goal. Most actions leave Value at [core.Static](0), so this
// reduces to goal value − cost in the common case.
func (p *Plan) NetValue(worldState core.WorldState) float64 {
	return p.Value(worldState) + p.ActionsValue(worldState) - p.Cost(worldState)
}

// SortByNetValueDesc sorts plans in place by NetValue descending.
// NetValue is computed once per plan against ws (the standard
// "evaluate at plan-selection time" snapshot) and the cached keys
// drive a stable sort — so each plan's NetValue is touched once
// instead of O(n log n) times.
//
// Used by every planner's [Planner.PlansToGoals] to rank candidates;
// hoisted here so the three implementations don't drift on the
// (subtle) ranking semantics.
func SortByNetValueDesc(plans []*Plan, ws core.WorldState) {
	if len(plans) < 2 {
		return
	}
	type keyed struct {
		plan *Plan
		net  float64
	}
	ranked := make([]keyed, len(plans))
	for i, pl := range plans {
		ranked[i] = keyed{plan: pl, net: pl.NetValue(ws)}
	}
	// The switch comparator (not cmp.Compare) keeps NaN net values
	// "equal", so the stable sort leaves them in place.
	slices.SortStableFunc(ranked, func(a, b keyed) int {
		switch {
		case a.net > b.net:
			return -1
		case a.net < b.net:
			return 1
		}
		return 0
	})
	for i, k := range ranked {
		plans[i] = k.plan
	}
}
