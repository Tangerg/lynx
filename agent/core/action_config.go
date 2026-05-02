package core

import "reflect"

// ActionConfig is the optional configuration bundle for [NewAction]. Every
// field is optional — pass a zero ActionConfig{} when defaults suffice.
// Choosing a struct over the functional-options pattern avoids polluting the
// package namespace with ~20 `With…` constructors and lets defaults +
// validation live in one place ([applyDefaults]).
//
// Static cost defaults to 1.0 — the planner shouldn't accidentally treat
// real work as zero-cost, so leaving Cost at its zero value triggers the
// 1.0 fallback. Express a "free" action as Cost: 0.001.
type ActionConfig struct {
	// Description is the human-readable prose surfaced in tracing,
	// dashboards, and (when an action is exposed as a tool) the LLM
	// prompt.
	Description string

	// Pre adds explicit precondition keys on top of the auto-derived
	// "input binding present" preconditions. Use for named boolean
	// conditions like "user_authenticated".
	Pre []string

	// Post mirrors Pre on the effects side: named conditions the action
	// establishes when it succeeds.
	Post []string

	// CanRerun lifts the default once-per-process restriction.
	CanRerun bool

	// ReadOnly marks an action as side-effect-free; the planner is free
	// to reorder or repeat it without worrying about state changes.
	ReadOnly bool

	// QoS overrides the default retry/back-off policy. Zero value falls
	// back to [DefaultActionQos].
	QoS ActionQos

	// Cost is the static planning cost. Zero falls back to 1.0.
	// Mutually exclusive with CostFn — when both are set CostFn wins.
	Cost float64

	// Value is the static planning value. Mutually exclusive with
	// ValueFn — when both are set ValueFn wins.
	Value float64

	// CostFn installs a state-dependent cost function. When set, it
	// overrides Cost during planning.
	CostFn CostFunc

	// ValueFn installs a state-dependent value function. When set, it
	// overrides Value during planning.
	ValueFn CostFunc

	// ToolGroups declares the abstract tool requirements (role names) —
	// the resolver translates these to concrete tools at execution
	// time.
	ToolGroups []ToolGroupRequirement

	// Trigger registers a "fire when this type appears" auto-action: if
	// the trigger type lands on the blackboard, the planner pulls this
	// action in regardless of whether it's on the current plan. Use
	// [TriggerType] to populate this from a Go type parameter.
	Trigger reflect.Type

	// OutputBinding overrides the default "it" output variable name.
	// Use when an action produces multiple distinct artifacts of the
	// same type.
	OutputBinding string

	// InputBinding mirrors OutputBinding for the single input binding.
	InputBinding string

	// Inputs replaces the default single-input binding with the
	// supplied list. Used when an action needs multiple distinct named
	// inputs (akin to embabel's @RequireNameMatch).
	Inputs []IoBinding

	// Outputs adds extra output bindings beyond the default one. Rare;
	// most actions produce a single canonical artifact.
	Outputs []IoBinding

	// ClearBlackboard marks the action as destructive — after it runs,
	// the blackboard is wiped (preserving "protected" entries).
	ClearBlackboard bool
}

// applyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver.
func (c *ActionConfig) applyDefaults() {
	if c.Cost == 0 {
		c.Cost = 1.0
	}
	if c.QoS == (ActionQos{}) {
		c.QoS = DefaultActionQos()
	}
}

// TriggerType returns a [reflect.Type] suitable for [ActionConfig.Trigger],
// derived from the type parameter T. Convenience wrapper that hides the
// `reflect.TypeFor[T]()` call from user code.
func TriggerType[T any]() reflect.Type { return reflect.TypeFor[T]() }

// ToolRolesFor builds a slice of [ToolGroupRequirement] from plain role
// names, defaulting each entry to action-scope termination. Convenience
// when you don't need per-role customization.
func ToolRolesFor(roles ...string) []ToolGroupRequirement {
	out := make([]ToolGroupRequirement, 0, len(roles))
	for _, role := range roles {
		out = append(out, ToolGroupRequirement{
			Role:             role,
			TerminationScope: TerminationScopeAction,
		})
	}
	return out
}
