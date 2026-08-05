package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Action is the agent's smallest planning unit. Implementations are
// typically produced via [NewAction] so the framework keeps type
// information end-to-end; the interface form is here for advanced users
// who want hand-rolled control over Execute (e.g. plugging into
// non-typed integrations).
type Action interface {
	Metadata() ActionMetadata
	// Execute runs the action body. Status models lifecycle outcomes such as
	// waiting or pausing; error carries failure detail and replan requests.
	Execute(ctx context.Context, process *ProcessContext) (ActionStatus, error)
}

// ActionMetadata is everything the planner needs to reason about an action
// without invoking it. Implementations must return a stable value; callers
// retaining it across a trust boundary should use [ActionMetadata.Clone].
//
// Cost and Value are [ScoreFunc]s rather than (static, fn) pairs so the
// planner has one uniform invocation point. Use [FixedScore] to lift a
// constant — e.g. `Cost: core.FixedScore(1.0)` — when no state-dependent
// math is needed.
type ActionMetadata struct {
	Name          string
	Description   string
	Inputs        []Binding
	Outputs       []Binding
	Preconditions ConditionSet
	Effects       ConditionSet
	Repeatable    bool
	ToolRoles     []string

	// Cost defaults to [FixedScore](1.0) so the planner doesn't pick
	// "free" actions over ones with real work.
	Cost ScoreFunc

	// Value defaults to [FixedScore](0).
	Value ScoreFunc

	ClearWorkingState bool // On success, clear working state before binding output.
}

// Clone returns an independent copy of every map and slice in m. Function
// values cannot be cloned and remain shared.
func (m ActionMetadata) Clone() ActionMetadata {
	m.Inputs = slices.Clone(m.Inputs)
	m.Outputs = slices.Clone(m.Outputs)
	m.Preconditions = maps.Clone(m.Preconditions)
	m.Effects = maps.Clone(m.Effects)
	m.ToolRoles = slices.Clone(m.ToolRoles)
	return m
}

// ActionDescriptor is the immutable, non-executable projection of an action.
// Planners consume [ActionMetadata], which includes dynamic score functions;
// observers and middleware receive this narrower value so they cannot execute
// the action or invoke planner policy.
type ActionDescriptor struct {
	name              string
	description       string
	inputs            []Binding
	outputs           []Binding
	preconditions     ConditionSet
	effects           ConditionSet
	repeatable        bool
	toolRoles         []string
	clearWorkingState bool
}

// Descriptor projects planner metadata into an inert value.
func (m ActionMetadata) Descriptor() ActionDescriptor {
	return ActionDescriptor{
		name:              m.Name,
		description:       m.Description,
		inputs:            slices.Clone(m.Inputs),
		outputs:           slices.Clone(m.Outputs),
		preconditions:     maps.Clone(m.Preconditions),
		effects:           maps.Clone(m.Effects),
		repeatable:        m.Repeatable,
		toolRoles:         slices.Clone(m.ToolRoles),
		clearWorkingState: m.ClearWorkingState,
	}
}

// Name returns the action's identity.
func (d ActionDescriptor) Name() string { return d.name }

// Description returns the caller-supplied human-readable purpose.
func (d ActionDescriptor) Description() string { return d.description }

// Inputs returns an independent snapshot of the action's inputs.
func (d ActionDescriptor) Inputs() []Binding { return slices.Clone(d.inputs) }

// Outputs returns an independent snapshot of the action's outputs.
func (d ActionDescriptor) Outputs() []Binding { return slices.Clone(d.outputs) }

// Preconditions returns an independent snapshot of the action's requirements.
func (d ActionDescriptor) Preconditions() ConditionSet { return maps.Clone(d.preconditions) }

// Effects returns an independent snapshot of the action's declared effects.
func (d ActionDescriptor) Effects() ConditionSet { return maps.Clone(d.effects) }

// Repeatable reports whether the planner may select the action more than once.
func (d ActionDescriptor) Repeatable() bool { return d.repeatable }

// ToolRoles returns an independent snapshot of the action's abstract tool roles.
func (d ActionDescriptor) ToolRoles() []string { return slices.Clone(d.toolRoles) }

// ClearsWorkingState reports whether success resets process working state.
func (d ActionDescriptor) ClearsWorkingState() bool { return d.clearWorkingState }

// actionSuccessConditionPrefix prefixes the conventional "this action has
// succeeded" condition keys minted by [ActionMetadata.SuccessCondition].
const actionSuccessConditionPrefix = "action_succeeded_"

// SuccessCondition is the conventional condition key recording that this
// action has succeeded at least once. The runtime sets it after each
// successful run; the planner consumes it as a precondition guard for
// non-rerunnable actions.
func (m ActionMetadata) SuccessCondition() string {
	return actionSuccessConditionPrefix + m.Name
}

// Applicable reports whether every precondition holds in state.
// It compares only the supplied truth map and never evaluates Unknown named
// conditions. Callers with evaluator-backed conditions must resolve them
// before using this snapshot-only helper.
func (m ActionMetadata) Applicable(state ConditionSet) bool {
	return state.Satisfies(m.Preconditions)
}

func (m ActionMetadata) validate() error {
	var problems []error
	for index, binding := range m.Inputs {
		if err := binding.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("input binding %d: %w", index, err))
		}
	}
	for index, binding := range m.Outputs {
		if err := binding.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("output binding %d: %w", index, err))
		}
	}
	if err := m.Preconditions.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("preconditions: %w", err))
	}
	if err := m.Effects.Validate(); err != nil {
		problems = append(problems, fmt.Errorf("effects: %w", err))
	}
	seenToolRoles := make(map[string]struct{}, len(m.ToolRoles))
	for index, role := range m.ToolRoles {
		switch {
		case role == "":
			problems = append(problems, fmt.Errorf("tool role %d: value is empty", index))
		case strings.TrimSpace(role) != role:
			problems = append(problems, fmt.Errorf("tool role %d: value has surrounding whitespace", index))
		default:
			if _, duplicate := seenToolRoles[role]; duplicate {
				problems = append(problems, fmt.Errorf("tool role %d: duplicate value %q", index, role))
			}
			seenToolRoles[role] = struct{}{}
		}
	}
	return errors.Join(problems...)
}

// ActionConfig is the optional configuration bundle for [NewAction].
// Every field is optional — pass a zero ActionConfig{} when defaults
// suffice. Choosing a struct over functional options keeps defaults
// and validation in one place.
//
// Cost and Value are [ScoreFunc]s rather than (static, fn) pairs. Use
// [FixedScore] to lift a constant. Leave Cost nil to inherit
// [FixedScore](1.0); leave Value nil for [FixedScore](0).
type ActionConfig struct {
	// Description surfaces in tracing, dashboards, and (when an
	// action is exposed as a tool) the LLM prompt.
	Description string

	// Preconditions adds explicit condition keys on top of the auto-derived
	// "input binding present" preconditions. Use for named boolean
	// conditions like "user_authenticated".
	Preconditions []string

	// Effects lists named conditions the action establishes on success.
	Effects []string

	// Repeatable allows the planner to select the action more than once in one
	// process. The zero value preserves once-per-process execution.
	Repeatable bool

	// Cost is the per-tick planning cost probe; nil means [FixedScore](1.0).
	Cost ScoreFunc

	// Value is the per-tick planning value probe; nil means [FixedScore](0).
	Value ScoreFunc

	// ToolRoles declares abstract roles — the resolver translates them
	// to concrete tools at execution time. Action bodies fetch the resolved tools via
	// [ProcessContext.ActionTools].
	ToolRoles []string

	// Inputs replaces the default single-input binding with the
	// supplied list. Use [NewBinding] to assign a non-default name or
	// declare multiple distinct inputs.
	Inputs []Binding

	// Outputs replaces the default single-output binding with the supplied
	// list. Use [NewBinding] to assign a non-default name. Rare; most actions
	// produce a single canonical artifact.
	Outputs []Binding

	// ClearWorkingState removes all blackboard bindings, objects, conditions,
	// and hidden markers on action success before binding the output. Useful for
	// state-machine transitions that intentionally discard prior working state.
	ClearWorkingState bool
}
