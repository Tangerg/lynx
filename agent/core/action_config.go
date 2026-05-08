package core

// ActionConfig is the optional configuration bundle for [NewAction].
// Every field is optional — pass a zero ActionConfig{} when defaults
// suffice. Choosing a struct over functional options keeps defaults
// + validation in one place.
//
// Cost / Value are [CostFunc]s rather than (static, fn) pairs. Use
// [Static] to lift a constant. Leave Cost nil to inherit
// [Static](1.0); leave Value nil for [Static](0). The planner shouldn't
// accidentally treat real work as zero-cost, so a nil Cost falls back
// to 1.0 rather than 0.
type ActionConfig struct {
	// Description surfaces in tracing, dashboards, and (when an
	// action is exposed as a tool) the LLM prompt.
	Description string

	// Pre adds explicit precondition keys on top of the auto-derived
	// "input binding present" preconditions. Use for named boolean
	// conditions like "user_authenticated".
	Pre []string

	// Post mirrors Pre on the effects side: named conditions the
	// action establishes on success.
	Post []string

	// CanRerun lifts the default once-per-process restriction.
	CanRerun bool

	// QoS overrides the default retry/back-off policy. Zero falls
	// back to [DefaultActionQoS].
	QoS ActionQoS

	// Cost is the per-tick planning cost probe; nil → [Static](1.0).
	Cost CostFunc

	// Value is the per-tick planning value probe; nil → [Static](0).
	Value CostFunc

	// ToolGroups declares the abstract tool requirements (role
	// names) — the resolver translates these to concrete tools at
	// execution time. Action bodies fetch the resolved tools via
	// [ProcessContext.ActionTools].
	ToolGroups []ToolGroupRequirement

	// OutputBinding overrides the default "it" output variable name.
	// Use when an action produces multiple distinct artifacts of
	// the same type.
	OutputBinding string

	// InputBinding mirrors OutputBinding for the single input
	// binding.
	InputBinding string

	// Inputs replaces the default single-input binding with the
	// supplied list. Used when an action needs multiple distinct
	// named inputs.
	Inputs []IOBinding

	// Outputs adds extra output bindings beyond the default one.
	// Rare; most actions produce a single canonical artifact.
	Outputs []IOBinding

	// ClearBlackboard wipes the blackboard (preserving protected
	// entries) on action success, before the output is bound.
	// Useful for state-machine transitions.
	ClearBlackboard bool
}

// applyDefaults fills in zero-valued fields whose conceptual default
// is non-zero.
func (c *ActionConfig) applyDefaults() {
	if c.Cost == nil {
		c.Cost = Static(1.0)
	}
	if c.Value == nil {
		c.Value = Static(0)
	}
	if c.QoS == (ActionQoS{}) {
		c.QoS = DefaultActionQoS()
	}
}

// ToolRolesFor builds a slice of [ToolGroupRequirement] from plain
// role names, defaulting each entry to action-scope termination.
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
