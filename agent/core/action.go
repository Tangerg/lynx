package core

import (
	"context"
	"reflect"
)

// Action is the agent's smallest planning unit. Implementations are typically
// produced via NewAction[In, Out] (see action_typed.go) so the framework keeps
// type information end-to-end; the interface form is here for advanced users
// who want hand-rolled control or for the reflect-registration layer.
type Action interface {
	Metadata() ActionMetadata
	// Execute runs the action body. It returns ActionStatus instead of an
	// error directly because some non-success outcomes (waiting, paused) are
	// not failures and the runtime needs to distinguish them.
	Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

// ActionMetadata is everything the planner needs to reason about an action
// without invoking it. It is intended to be immutable after construction.
type ActionMetadata struct {
	Name          string
	Description   string
	Inputs        []IoBinding
	Outputs       []IoBinding
	Preconditions EffectSpec
	Effects       EffectSpec
	CanRerun      bool
	ReadOnly      bool
	QoS           ActionQos
	ToolGroups    []ToolGroupRequirement

	// CostFn is the optional dynamic cost. CostStatic is the fallback when
	// CostFn is nil — both default to 1.0 so the planner doesn't accidentally
	// pick "free" actions in preference to ones that have real work to do.
	CostFn      CostFunc
	ValueFn     CostFunc
	CostStatic  float64
	ValueStatic float64

	Trigger         reflect.Type // Optional — autostart this action when the trigger type appears.
	OutputBinding   string       // Override the variable name written to the blackboard.
	ClearBlackboard bool         // Destructive; planner treats as terminal.
}

// Cost resolves the (CostFn or CostStatic) pair. The runtime uses this rather
// than reading the fields directly so the precedence rule lives in one place.
func (m ActionMetadata) Cost(ws WorldState) float64 {
	if m.CostFn != nil {
		return m.CostFn(ws)
	}
	if m.CostStatic == 0 {
		return 1.0
	}
	return m.CostStatic
}

// Value resolves the (ValueFn or ValueStatic) pair. Goal pursuit subtracts the
// total plan cost from this; high-value actions appear earlier in plans.
func (m ActionMetadata) Value(ws WorldState) float64 {
	if m.ValueFn != nil {
		return m.ValueFn(ws)
	}
	return m.ValueStatic
}

// HasRunKey is the conventional condition key recording that this action has
// executed at least once. The runtime sets it after each successful run; the
// planner consumes it as a precondition guard for non-rerunnable actions.
func (m ActionMetadata) HasRunKey() string {
	return "hasRun_" + m.Name
}

// IsApplicableIn reports whether every precondition holds in state. Used by
// the concurrent runner to filter the plan's actions to those currently
// runnable on this tick.
func (m ActionMetadata) IsApplicableIn(state map[string]Determination) bool {
	for key, required := range m.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}
