package core

import "reflect"

// ActionConfig is the optional configuration bundle for [NewAction].
// Every field is optional — pass a zero ActionConfig{} when defaults
// suffice. Choosing a struct over functional options keeps defaults +
// validation in one place ([applyDefaults]).
//
// Cost and Value are [CostFunc]s rather than (static, fn) pairs. Use
// [Static] to lift a constant — e.g. `Cost: core.Static(2.5)`. Leave
// Cost nil to inherit the [Static](1.0) default; leave Value nil for
// [Static](0). The planner shouldn't accidentally treat real work as
// zero-cost, so a nil Cost falls back to 1.0 rather than 0.
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
	//
	// TODO(future): not consumed by the planner today (the heuristic
	// doesn't exploit reordering). Reserved for a future planner
	// optimisation pass.
	ReadOnly bool

	// QoS overrides the default retry/back-off policy. Zero value falls
	// back to [DefaultActionQos].
	QoS ActionQos

	// Cost is the per-tick planning cost probe. Nil falls back to
	// [Static](1.0).
	Cost CostFunc

	// Value is the per-tick planning value probe. Nil falls back to
	// [Static](0).
	Value CostFunc

	// ToolGroups declares the abstract tool requirements (role names) —
	// the resolver translates these to concrete tools at execution
	// time. Action bodies can call [ProcessContext.ActionTools] to
	// fetch the resolved tools without re-stating the role names.
	ToolGroups []ToolGroupRequirement

	// Trigger registers a "fire when this type appears" auto-action: if
	// the trigger type lands on the blackboard, the planner pulls this
	// action in regardless of whether it's on the current plan. Use
	// [TriggerType] to populate this from a Go type parameter.
	//
	// TODO(future): stored on ActionMetadata.Trigger but the planner
	// does NOT scan for matching types today. Reserved.
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
	Inputs []IOBinding

	// Outputs adds extra output bindings beyond the default one. Rare;
	// most actions produce a single canonical artifact.
	Outputs []IOBinding

	// ClearBlackboard wipes the blackboard (preserving protected
	// entries) on action success, before the output is bound. Useful
	// for state-machine transitions and looping flows where stale named
	// values would confuse the next planning tick. Mirrors embabel's
	// @Action(clearBlackboard = true).
	ClearBlackboard bool
}

// applyDefaults fills in zero-valued fields whose conceptual default is
// non-zero. Mutates the receiver.
func (c *ActionConfig) applyDefaults() {
	if c.Cost == nil {
		c.Cost = Static(1.0)
	}
	if c.Value == nil {
		c.Value = Static(0)
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
