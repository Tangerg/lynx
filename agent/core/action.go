package core

import (
	"context"
	"reflect"
)

// Action is the agent's smallest planning unit. Implementations are
// typically produced via [NewAction] so the framework keeps type
// information end-to-end; the interface form is here for advanced users
// who want hand-rolled control over Execute (e.g. plugging into
// non-typed integrations).
type Action interface {
	Metadata() ActionMetadata
	// Execute runs the action body. It returns ActionStatus instead of an
	// error directly because some non-success outcomes (waiting, paused) are
	// not failures and the runtime needs to distinguish them.
	Execute(ctx context.Context, pc *ProcessContext) ActionStatus
}

// ActionMetadata is everything the planner needs to reason about an
// action without invoking it. Immutable after construction.
//
// Cost and Value are [CostFunc]s rather than (static, fn) pairs so the
// planner has one uniform invocation point. Use [Static] to lift a
// constant — e.g. `Cost: core.Static(1.0)` — when no state-dependent
// math is needed.
type ActionMetadata struct {
	Name          string
	Description   string
	Inputs        []IOBinding
	Outputs       []IOBinding
	Preconditions EffectSpec
	Effects       EffectSpec
	CanRerun      bool
	ReadOnly      bool
	QoS           ActionQos
	ToolGroups    []ToolGroupRequirement

	// Cost is the planner's per-tick cost probe; defaults to
	// [Static](1.0) so the planner doesn't accidentally pick "free"
	// actions in preference to ones with real work to do.
	Cost CostFunc

	// Value is the planner's per-tick value probe; defaults to
	// [Static](0).
	Value CostFunc

	Trigger         reflect.Type // Optional — autostart this action when the trigger type appears.
	OutputBinding   string       // Override the variable name written to the blackboard.
	ClearBlackboard bool         // On success, clear blackboard before binding output.
}

// HasRunKey is the conventional condition key recording that this
// action has executed at least once. The runtime sets it after each
// successful run; the planner consumes it as a precondition guard for
// non-rerunnable actions.
func (m ActionMetadata) HasRunKey() string {
	return "hasRun_" + m.Name
}

// IsApplicableIn reports whether every precondition holds in state.
// Used by the concurrent runner to filter the plan's actions to those
// currently runnable on this tick.
func (m ActionMetadata) IsApplicableIn(state map[string]Determination) bool {
	for key, required := range m.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}
