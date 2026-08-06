package planning

import (
	"fmt"
	"math"
	"slices"
)

// Problem is one immutable Planner input: a current observation, one Goal, and
// the available predictive Actions. It contains no dispatcher, Process, or
// application dependency.
type Problem struct {
	initial WorldState
	goal    Goal
	actions []Action
}

// NewProblem validates and freezes one Planning problem. Action names must be
// unique in the problem.
func NewProblem(initial WorldState, goal Goal, actions ...Action) (Problem, error) {
	if !initial.Valid() {
		return Problem{}, fmt.Errorf("%w: initial state", ErrInvalidProblem)
	}
	if !goal.Valid() {
		return Problem{}, fmt.Errorf("%w: Goal", ErrInvalidProblem)
	}
	values := slices.Clone(actions)
	seen := make(map[string]struct{}, len(values))
	for index, action := range values {
		if !action.Valid() {
			return Problem{}, fmt.Errorf("%w: Action %d", ErrInvalidProblem, index)
		}
		if _, duplicate := seen[action.name]; duplicate {
			return Problem{}, fmt.Errorf("%w: duplicate Action %q", ErrInvalidProblem, action.name)
		}
		seen[action.name] = struct{}{}
	}
	return Problem{initial: initial, goal: goal, actions: values}, nil
}

// InitialState returns the immutable starting observation.
func (problem Problem) InitialState() WorldState { return problem.initial }

// Goal returns the immutable desired state.
func (problem Problem) Goal() Goal { return problem.goal }

// Actions returns an independently owned slice in declaration order.
func (problem Problem) Actions() []Action { return slices.Clone(problem.actions) }

// Action returns the named Action and true, or the zero Action and false.
func (problem Problem) Action(name string) (Action, bool) {
	for _, action := range problem.actions {
		if action.name == name {
			return action, true
		}
	}
	return Action{}, false
}

// Valid reports whether the Problem satisfies its construction invariants.
func (problem Problem) Valid() bool {
	if !problem.initial.Valid() || !problem.goal.Valid() {
		return false
	}
	seen := make(map[string]struct{}, len(problem.actions))
	for _, action := range problem.actions {
		if !action.Valid() {
			return false
		}
		if _, duplicate := seen[action.name]; duplicate {
			return false
		}
		seen[action.name] = struct{}{}
	}
	return true
}

// ValidatePlan verifies that every referenced Action exists and is applicable
// in sequence, the reported cost equals the evaluated path cost, and the
// predicted final state satisfies the Goal.
func (problem Problem) ValidatePlan(plan Plan) error {
	if !problem.Valid() || !plan.Valid() {
		return ErrInvalidPlan
	}
	state := problem.initial
	totalCost := 0.0
	for index, planned := range plan.actions {
		action, found := problem.Action(planned.name)
		if !found {
			return fmt.Errorf("%w: Action %d references unknown %q", ErrInvalidPlan, index, planned.name)
		}
		if !action.Applicable(state) {
			return fmt.Errorf("%w: Action %q is not applicable at step %d", ErrInvalidPlan, planned.name, index)
		}
		cost, err := action.Cost(state)
		if err != nil {
			return err
		}
		totalCost += cost
		if math.IsInf(totalCost, 0) {
			return fmt.Errorf("%w: total cost overflow", ErrInvalidPlan)
		}
		state, err = action.Apply(state)
		if err != nil {
			return fmt.Errorf("%w: apply Action %q: %w", ErrInvalidPlan, planned.name, err)
		}
	}
	if totalCost != plan.totalCost {
		return fmt.Errorf("%w: total cost %v does not equal evaluated cost %v", ErrInvalidPlan, plan.totalCost, totalCost)
	}
	if !problem.goal.SatisfiedBy(state) {
		return fmt.Errorf("%w: predicted final state does not satisfy Goal %q", ErrInvalidPlan, problem.goal.name)
	}
	return nil
}
