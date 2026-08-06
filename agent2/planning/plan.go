package planning

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

// PlannedAction is one immutable Action reference in Planner-selected order.
// It contains no executable capability or copied Action metadata.
type PlannedAction struct {
	name string
}

// NewPlannedAction constructs a stable reference to a named Action.
func NewPlannedAction(name string) (PlannedAction, error) {
	if !validName(name) {
		return PlannedAction{}, fmt.Errorf("%w: invalid Action name %q", ErrInvalidPlan, name)
	}
	return PlannedAction{name: name}, nil
}

// Name returns the referenced Action identity.
func (action PlannedAction) Name() string { return action.name }

// Valid reports whether the reference contains a valid Action identity.
func (action PlannedAction) Valid() bool { return validName(action.name) }

func (action PlannedAction) MarshalJSON() ([]byte, error) {
	if !action.Valid() {
		return nil, ErrInvalidPlan
	}
	return json.Marshal(action.name)
}

func (action *PlannedAction) UnmarshalJSON(data []byte) error {
	if action == nil {
		return fmt.Errorf("%w: nil PlannedAction receiver", ErrInvalidPlan)
	}
	var name string
	if err := decodeStrict(data, &name); err != nil {
		return fmt.Errorf("%w: decode PlannedAction: %w", ErrInvalidPlan, err)
	}
	value, err := NewPlannedAction(name)
	if err != nil {
		return err
	}
	*action = value
	return nil
}

// Plan is an immutable ordered Action sequence and its predicted total cost.
// An empty Plan with zero cost is valid and represents an already-satisfied
// Goal; Planner's separate found result distinguishes it from no solution.
type Plan struct {
	actions   []PlannedAction
	totalCost float64
}

// NewPlan validates and freezes a Planner result.
func NewPlan(actions []PlannedAction, totalCost float64) (Plan, error) {
	if math.IsNaN(totalCost) || math.IsInf(totalCost, 0) || totalCost < 0 {
		return Plan{}, fmt.Errorf("%w: invalid total cost %v", ErrInvalidPlan, totalCost)
	}
	values := slices.Clone(actions)
	for index, action := range values {
		if !action.Valid() {
			return Plan{}, fmt.Errorf("%w: Action %d", ErrInvalidPlan, index)
		}
	}
	if len(values) == 0 && totalCost != 0 {
		return Plan{}, fmt.Errorf("%w: empty Plan must have zero cost", ErrInvalidPlan)
	}
	return Plan{actions: values, totalCost: totalCost}, nil
}

// Actions returns independently owned Action references in execution order.
func (plan Plan) Actions() []PlannedAction { return slices.Clone(plan.actions) }

// TotalCost returns the predicted sum of Action edge costs.
func (plan Plan) TotalCost() float64 { return plan.totalCost }

// Valid reports whether every reference and the total cost are valid.
func (plan Plan) Valid() bool {
	if math.IsNaN(plan.totalCost) || math.IsInf(plan.totalCost, 0) || plan.totalCost < 0 ||
		len(plan.actions) == 0 && plan.totalCost != 0 {
		return false
	}
	for _, action := range plan.actions {
		if !action.Valid() {
			return false
		}
	}
	return true
}

func (plan Plan) MarshalJSON() ([]byte, error) {
	if !plan.Valid() {
		return nil, ErrInvalidPlan
	}
	actions := slices.Clone(plan.actions)
	if actions == nil {
		actions = []PlannedAction{}
	}
	return json.Marshal(planWire{Actions: actions, TotalCost: plan.totalCost})
}

func (plan *Plan) UnmarshalJSON(data []byte) error {
	if plan == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidPlan)
	}
	var wire planWire
	if err := decodeStrict(data, &wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidPlan, err)
	}
	value, err := NewPlan(wire.Actions, wire.TotalCost)
	if err != nil {
		return err
	}
	*plan = value
	return nil
}

type planWire struct {
	Actions   []PlannedAction `json:"actions"`
	TotalCost float64         `json:"total_cost"`
}
