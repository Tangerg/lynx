package core

import "slices"

// ActionConfig is the optional configuration bundle for [NewAction].
// Every field is optional — pass a zero ActionConfig{} when defaults
// suffice. Choosing a struct over functional options keeps defaults
// + validation in one place.
//
// Cost / Value are [ScoreFunc]s rather than (static, fn) pairs. Use
// [FixedScore] to lift a constant. Leave Cost nil to inherit
// [FixedScore](1.0); leave Value nil for [FixedScore](0). The planner shouldn't
// accidentally treat real work as zero-cost, so a nil Cost falls back
// to 1.0 rather than 0.
type ActionConfig struct {
	// Description surfaces in tracing, dashboards, and (when an
	// action is exposed as a tool) the LLM prompt.
	Description string

	// Preconditions adds explicit condition keys on top of the auto-derived
	// "input binding present" preconditions. Use for named boolean
	// conditions like "user_authenticated".
	Preconditions []string

	// Effects lists named conditions the
	// action establishes on success.
	Effects []string

	// Repeatable allows the planner to select the action more than once in one
	// process. The zero value preserves once-per-process execution.
	Repeatable bool

	// Retry explicitly opts this action into replay after failure. The zero
	// value means one attempt. MaxAttempts above one also requires a Safety
	// declaration; invalid policies are rejected when the Agent is deployed.
	Retry RetryPolicy

	// Cost is the per-tick planning cost probe; nil → [FixedScore](1.0).
	Cost ScoreFunc

	// Value is the per-tick planning value probe; nil → [FixedScore](0).
	Value ScoreFunc

	// ToolGroups declares the abstract tool requirements (role
	// names) — the resolver translates these to concrete tools at
	// execution time. Action bodies fetch the resolved tools via
	// [ProcessContext.ActionTools].
	ToolGroups []ToolGroupRequirement

	// Inputs replaces the default single-input binding with the
	// supplied list. Use [NewBinding] to assign a non-default name or
	// declare multiple distinct inputs.
	Inputs []Binding

	// Outputs replaces the default single-output binding with the supplied
	// list. Use [NewBinding] to assign a non-default name. Rare; most actions
	// produce a single canonical artifact.
	Outputs []Binding

	// ClearWorkingState removes ordinary blackboard state on action success
	// before binding the output. Protected ambient entries remain available.
	// Useful for state-machine transitions.
	ClearWorkingState bool
}

// RequireToolGroup declares one role and the permissions an action is willing
// to grant it. Omitting permissions keeps the requirement unprivileged.
func RequireToolGroup(role string, allowed ...ToolGroupPermission) ToolGroupRequirement {
	return ToolGroupRequirement{Role: role, AllowedPermissions: slices.Clone(allowed)}
}
