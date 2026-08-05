package planning

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/agent/internal/score"
)

// Plan is a planner output: an ordered action chain whose accumulated effects
// achieve its goal. It owns the chain's slice storage but retains the action
// capabilities. [Domain.Plans] validates and replaces candidates with the
// Domain's canonical actions before returning a plan to runtime. An empty
// chain with a non-nil goal means the goal is already satisfied.
type Plan struct {
	actions []core.Action
	goal    *core.Goal
}

// PlanDescriptor is the immutable, non-executable projection of a plan.
// It is the observation shape; the executable Plan remains private to planning
// and runtime control flow.
type PlanDescriptor struct {
	actions []core.ActionDescriptor
	goal    core.GoalDescriptor
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

// Descriptor projects a plan without exposing executable actions or score
// functions.
func (p *Plan) Descriptor() PlanDescriptor {
	if p == nil {
		return PlanDescriptor{}
	}
	actions := make([]core.ActionDescriptor, len(p.actions))
	for index, action := range p.actions {
		if !nilvalue.Is(action) {
			actions[index] = action.Metadata().Descriptor()
		}
	}
	return PlanDescriptor{actions: actions, goal: p.goal.Descriptor()}
}

// Actions returns an independent snapshot of the ordered action descriptions.
func (d PlanDescriptor) Actions() []core.ActionDescriptor {
	return slices.Clone(d.actions)
}

// Goal returns the plan's inert target description.
func (d PlanDescriptor) Goal() core.GoalDescriptor { return d.goal }

// Complete reports whether the descriptor contains no action to execute.
func (d PlanDescriptor) Complete() bool { return len(d.actions) == 0 }

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
		if nilvalue.Is(action) {
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
		if nilvalue.Is(action) {
			continue
		}
		if fn := action.Metadata().Value; fn != nil {
			total += fn(worldState)
		}
	}
	return total
}

// NetValue ranks competing plans as goal value plus [Plan.ActionsValue] minus
// [Plan.Cost]. The actions term is what stops the ranking from always preferring
// the cheapest path to a goal; most actions leave Value at
// [core.FixedScore](0), so it contributes nothing unless an author opts in.
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
	goalValue := 0.0
	if p.goal != nil {
		var err error
		goalValue, err = score.Evaluate(func(state core.WorldState) float64 { return p.goal.Value(state) }, worldState)
		if err != nil {
			return 0, fmt.Errorf("planning: goal %q value: %w", p.goal.Name(), err)
		}
		if !score.Finite(goalValue) {
			return 0, fmt.Errorf("planning: goal %q value returned %v", p.goal.Name(), goalValue)
		}
	}
	total := goalValue
	for _, action := range p.actions {
		if nilvalue.Is(action) {
			continue
		}
		metadata := action.Metadata()
		if metadata.Value != nil {
			value, err := score.Evaluate(metadata.Value, worldState)
			if err != nil {
				return 0, fmt.Errorf("planning: action %q value: %w", metadata.Name, err)
			}
			if !score.Finite(value) {
				return 0, fmt.Errorf("planning: action %q value returned %v", metadata.Name, value)
			}
			total += value
		}
		if metadata.Cost != nil {
			cost, err := score.Evaluate(metadata.Cost, worldState)
			if err != nil {
				return 0, fmt.Errorf("planning: action %q cost: %w", metadata.Name, err)
			}
			if !score.Finite(cost) || cost < 0 {
				return 0, fmt.Errorf("planning: action %q cost returned %v; cost must be finite and non-negative", metadata.Name, cost)
			}
			total -= cost
		}
	}
	if !score.Finite(total) {
		goalName := "<nil>"
		if p.goal != nil {
			goalName = p.goal.Name()
		}
		return 0, fmt.Errorf("planning: plan for goal %q net value overflowed to %v", goalName, total)
	}
	return total, nil
}
