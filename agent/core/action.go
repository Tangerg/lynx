package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
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

// ActionMetadata is everything the planner needs to reason about an
// action without invoking it. Immutable after construction.
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
	Retry         RetryPolicy
	ToolGroups    []ToolGroupRequirement

	// Cost defaults to [FixedScore](1.0) so the planner doesn't pick
	// "free" actions over ones with real work.
	Cost ScoreFunc

	// Value defaults to [FixedScore](0).
	Value ScoreFunc

	ClearWorkingState bool // On success, clear working state before binding output.
}

func (m ActionMetadata) clone() ActionMetadata {
	m.Inputs = slices.Clone(m.Inputs)
	m.Outputs = slices.Clone(m.Outputs)
	m.Preconditions = maps.Clone(m.Preconditions)
	m.Effects = maps.Clone(m.Effects)
	m.ToolGroups = cloneToolGroupRequirements(m.ToolGroups)
	return m
}

func cloneToolGroupRequirements(requirements []ToolGroupRequirement) []ToolGroupRequirement {
	if requirements == nil {
		return nil
	}
	cloned := make([]ToolGroupRequirement, len(requirements))
	for i, requirement := range requirements {
		cloned[i] = requirement
		cloned[i].AllowedPermissions = slices.Clone(requirement.AllowedPermissions)
	}
	return cloned
}

// ActionRunConditionPrefix prefixes the conventional "this action has run" condition
// keys ([ActionMetadata.RunCondition] mints them; the runtime's world-
// state determiner routes any key with this prefix to the blackboard's
// condition map). One constant on the protocol type so the producer and
// the classifier can't drift.
const ActionRunConditionPrefix = "action_ran_"

// RunCondition is the conventional condition key recording that this
// action has executed at least once. The runtime sets it after each
// successful run; the planner consumes it as a precondition guard for
// non-rerunnable actions.
func (m ActionMetadata) RunCondition() string {
	return ActionRunConditionPrefix + m.Name
}

// Applicable reports whether every precondition holds in state.
// Used by the concurrent runner to filter the plan's actions to those
// currently runnable on this tick.
func (m ActionMetadata) Applicable(state map[string]Truth) bool {
	for key, required := range m.Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}

func (m ActionMetadata) validate() error {
	var problems []error
	if err := m.Retry.validate(); err != nil {
		problems = append(problems, fmt.Errorf("retry policy: %w", err))
	}
	for index, requirement := range m.ToolGroups {
		if err := requirement.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("tool group %d (%q): %w", index, requirement.Role, err))
		}
	}
	return errors.Join(problems...)
}

// RetrySafety states why replaying an action after failure is safe. It is a
// declaration by the action author, not an error classification performed by
// the framework.
type RetrySafety uint8

const (
	// RetrySafetyUnspecified is the zero value and permits only one attempt.
	RetrySafetyUnspecified RetrySafety = iota
	// RetrySafetyIdempotent means repeated execution with the same input has
	// the same externally observable effect as one execution.
	RetrySafetyIdempotent
	// RetrySafetyCompensated means a failed attempt has undone every external
	// effect before returning its error.
	RetrySafetyCompensated
)

func (s RetrySafety) String() string {
	switch s {
	case RetrySafetyUnspecified:
		return "unspecified"
	case RetrySafetyIdempotent:
		return "idempotent"
	case RetrySafetyCompensated:
		return "compensated"
	default:
		return "unknown"
	}
}

// Valid reports whether s is a framework-defined retry safety value.
func (s RetrySafety) Valid() bool {
	return s >= RetrySafetyUnspecified && s <= RetrySafetyCompensated
}

// RetryPolicy governs replay of one action. The zero value and
// [DefaultRetryPolicy] both mean one attempt. Setting MaxAttempts above one is
// rejected at deployment unless Safety explicitly declares why replay is
// safe. The framework never guesses from an error whether a side effect is
// retryable.
type RetryPolicy struct {
	// MaxAttempts caps total tries (initial + retries). 0 means one attempt.
	MaxAttempts int
	// BaseDelay is the initial wait; successive attempts grow ×2 up
	// to MaxDelay with jitter.
	BaseDelay time.Duration
	// MaxDelay caps the per-attempt wait. 0 means uncapped.
	MaxDelay time.Duration
	// Safety is required when MaxAttempts is greater than one.
	Safety RetrySafety
}

// DefaultRetryPolicy returns the side-effect-safe framework default: exactly
// one attempt and no replay declaration.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 1}
}

func (p RetryPolicy) validate() error {
	if p.MaxAttempts < 0 {
		return errors.New("max attempts must not be negative")
	}
	if p.BaseDelay < 0 || p.MaxDelay < 0 {
		return errors.New("retry delays must not be negative")
	}
	if p.MaxDelay > 0 && p.BaseDelay > p.MaxDelay {
		return fmt.Errorf("base delay %s exceeds max delay %s", p.BaseDelay, p.MaxDelay)
	}
	if !p.Safety.Valid() {
		return fmt.Errorf("unknown retry safety %d", p.Safety)
	}
	if p.MaxAttempts > 1 && p.Safety == RetrySafetyUnspecified {
		return fmt.Errorf("max attempts %d requires idempotent or compensated retry safety", p.MaxAttempts)
	}
	return nil
}

// ConditionSet maps condition keys to their required or produced truth values.
// It is used by action preconditions, action effects, goals, and world-state
// transitions. A nil set is valid and means no conditions.
type ConditionSet map[string]Truth

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

	// Retry explicitly opts this action into replay after failure. The zero
	// value means one attempt. MaxAttempts above one also requires a Safety
	// declaration; invalid policies are rejected when the Agent is deployed.
	Retry RetryPolicy

	// Cost is the per-tick planning cost probe; nil means [FixedScore](1.0).
	Cost ScoreFunc

	// Value is the per-tick planning value probe; nil means [FixedScore](0).
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
