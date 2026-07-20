package planning

import (
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// Plan is an immutable planner output: an ordered action chain whose
// accumulated effects achieve its goal. An empty chain with a non-nil goal
// means the goal is already satisfied.
type Plan struct {
	actions []core.Action
	goal    *core.Goal
}

// NewPlan constructs a complete planner result and snapshots the action chain.
func NewPlan(actions []core.Action, goal *core.Goal) *Plan {
	return &Plan{actions: slices.Clone(actions), goal: goal}
}

// Actions returns a snapshot of the ordered action chain.
func (p *Plan) Actions() []core.Action {
	if p == nil {
		return nil
	}
	return slices.Clone(p.actions)
}

// Goal returns the immutable target of the plan.
func (p *Plan) Goal() *core.Goal {
	if p == nil {
		return nil
	}
	return p.goal
}

// Complete reports whether no more work is needed for this goal.
func (p *Plan) Complete() bool {
	return p == nil || len(p.actions) == 0
}

// Cost is the sum of action costs; the planner uses it to rank competing
// plans. It samples each action's cost against the supplied world state so
// dynamic-cost actions get evaluated correctly. Actions with a nil Cost
// contribute nothing — the canonical construction path ([core.NewAction])
// fills in [core.FixedScore](1.0).
func (p *Plan) Cost(worldState core.WorldState) float64 {
	if p == nil {
		return 0
	}

	total := 0.0
	for _, action := range p.actions {
		if action == nil {
			continue
		}
		if fn := action.Metadata().Cost; fn != nil {
			total += fn(worldState)
		}
	}
	return total
}

// Value evaluates the goal value. A nil goal contributes zero.
func (p *Plan) Value(worldState core.WorldState) float64 {
	if p == nil || p.goal == nil {
		return 0
	}
	return p.goal.Value(worldState)
}

// ActionsValue is the sum of the plan's action values, sampled against the
// supplied world state so dynamic-value actions get evaluated correctly.
// Actions with a nil Value contribute nothing — the canonical construction
// path ([core.NewAction]) fills in [core.FixedScore](0), so this term is zero
// unless an action opts into a non-trivial value.
func (p *Plan) ActionsValue(worldState core.WorldState) float64 {
	if p == nil {
		return 0
	}

	total := 0.0
	for _, action := range p.actions {
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
// the plan's actions, minus total plan cost. Follows the standard plan-value
// pattern (goal.value + actionsValue − cost) — the actions-value term rewards
// plans whose constituent actions are independently valuable, not just the
// cheapest path to the goal. Most actions leave Value at [core.FixedScore](0),
// so this reduces to goal value − cost in the common case.
func (p *Plan) NetValue(worldState core.WorldState) float64 {
	return p.Value(worldState) + p.ActionsValue(worldState) - p.Cost(worldState)
}

// sortByNetValueDesc sorts plans in place by NetValue descending.
// NetValue is computed once per plan against worldState (the standard
// "evaluate at plan-selection time" snapshot) and the cached keys
// drive a stable sort — so each plan's NetValue is touched once
// instead of O(n log n) times.
//
// Used by [Domain.Plans] to rank candidates;
// hoisted here so the three implementations don't drift on the
// (subtle) ranking semantics.
func sortByNetValueDesc(plans []*Plan, worldState core.WorldState) error {
	if len(plans) < 2 {
		if len(plans) == 1 {
			_, err := plans[0].checkedNetValue(worldState)
			return err
		}
		return nil
	}
	type keyed struct {
		plan *Plan
		net  float64
	}
	ranked := make([]keyed, len(plans))
	for index, plan := range plans {
		net, err := plan.checkedNetValue(worldState)
		if err != nil {
			return err
		}
		ranked[index] = keyed{plan: plan, net: net}
	}
	slices.SortStableFunc(ranked, func(left, right keyed) int {
		switch {
		case left.net > right.net:
			return -1
		case left.net < right.net:
			return 1
		}
		return 0
	})
	for index, item := range ranked {
		plans[index] = item.plan
	}
	return nil
}

func (p *Plan) checkedNetValue(worldState core.WorldState) (float64, error) {
	if p == nil {
		return 0, nil
	}
	goalValue := 0.0
	if p.goal != nil {
		var err error
		goalValue, err = evaluatePlanScore(func(state core.WorldState) float64 { return p.goal.Value(state) }, worldState)
		if err != nil {
			return 0, fmt.Errorf("planning: goal %q value: %w", p.goal.Name(), err)
		}
		if !finite(goalValue) {
			return 0, fmt.Errorf("planning: goal %q value returned %v", p.goal.Name(), goalValue)
		}
	}
	total := goalValue
	for _, action := range p.actions {
		if action == nil {
			continue
		}
		metadata := action.Metadata()
		if metadata.Value != nil {
			value, err := evaluatePlanScore(metadata.Value, worldState)
			if err != nil {
				return 0, fmt.Errorf("planning: action %q value: %w", metadata.Name, err)
			}
			if !finite(value) {
				return 0, fmt.Errorf("planning: action %q value returned %v", metadata.Name, value)
			}
			total += value
		}
		if metadata.Cost != nil {
			cost, err := evaluatePlanScore(metadata.Cost, worldState)
			if err != nil {
				return 0, fmt.Errorf("planning: action %q cost: %w", metadata.Name, err)
			}
			if !finite(cost) || cost < 0 {
				return 0, fmt.Errorf("planning: action %q cost returned %v; cost must be finite and non-negative", metadata.Name, cost)
			}
			total -= cost
		}
	}
	if !finite(total) {
		goalName := "<nil>"
		if p.goal != nil {
			goalName = p.goal.Name()
		}
		return 0, fmt.Errorf("planning: plan for goal %q net value overflowed to %v", goalName, total)
	}
	return total, nil
}

func evaluatePlanScore(score core.ScoreFunc, state core.WorldState) (value float64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("score function panicked", recovered)
		}
	}()
	return score(state), nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
