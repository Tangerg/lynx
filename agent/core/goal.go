package core

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
)

const defaultGoalValue = 1.0

// GoalConfig is the construction input for [NewGoal]. It remains ordinary Go
// data; Goal takes a defensive snapshot and owns the resulting value object.
type GoalConfig struct {
	Name               string
	Description        string
	RequiredConditions []string
	RequiredBindings   []Binding

	// Value is the planner's per-tick value probe. [NewOutputGoal]
	// fills [FixedScore](1.0) when left nil.
	Value ScoreFunc
}

// Goal is an immutable target state. The planner finds an action sequence
// whose cumulative effects satisfy its requirements and ranks it using Value.
type Goal struct {
	name               string
	description        string
	requiredConditions []string
	requiredBindings   []Binding
	value              ScoreFunc
}

// GoalDescriptor is the immutable, non-executable projection of a planning
// target. It deliberately excludes Value: observers can describe a goal without
// invoking planner policy.
type GoalDescriptor struct {
	name               string
	description        string
	requiredConditions []string
	requiredBindings   []Binding
}

// NewGoal constructs an immutable goal from config.
func NewGoal(config GoalConfig) *Goal {
	return &Goal{
		name:               config.Name,
		description:        config.Description,
		requiredConditions: slices.Clone(config.RequiredConditions),
		requiredBindings:   slices.Clone(config.RequiredBindings),
		value:              config.Value,
	}
}

// Name identifies the goal. It answers for a nil goal so callers holding an
// unset or not-yet-selected goal can report on it without a guard of their own.
func (g *Goal) Name() string {
	if g == nil {
		return ""
	}
	return g.name
}

func (g *Goal) Description() string {
	if g == nil {
		return ""
	}
	return g.description
}

// Descriptor projects the goal into an inert value suitable for events.
func (g *Goal) Descriptor() GoalDescriptor {
	if g == nil {
		return GoalDescriptor{}
	}
	return GoalDescriptor{
		name:               g.name,
		description:        g.description,
		requiredConditions: slices.Clone(g.requiredConditions),
		requiredBindings:   slices.Clone(g.requiredBindings),
	}
}

// Name returns the goal's identity.
func (d GoalDescriptor) Name() string { return d.name }

// Description returns the caller-supplied human-readable purpose.
func (d GoalDescriptor) Description() string { return d.description }

// RequiredConditions returns an independent snapshot of the named requirements.
func (d GoalDescriptor) RequiredConditions() []string {
	return slices.Clone(d.requiredConditions)
}

// RequiredBindings returns an independent snapshot of the typed requirements.
func (d GoalDescriptor) RequiredBindings() []Binding { return slices.Clone(d.requiredBindings) }

func (g *Goal) validate() error {
	if g == nil {
		return errors.New("goal is nil")
	}
	var problems []error
	for index, condition := range g.requiredConditions {
		if err := validateConditionKey(condition); err != nil {
			problems = append(problems, fmt.Errorf("required condition %d: %w", index, err))
		}
	}
	for index, binding := range g.requiredBindings {
		if err := binding.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("required binding %d: %w", index, err))
		}
	}
	return errors.Join(problems...)
}

// RequiredConditions returns the explicitly named condition requirements.
func (g *Goal) RequiredConditions() []string {
	if g == nil {
		return nil
	}
	return slices.Clone(g.requiredConditions)
}

// RequiredBindings returns the typed bindings required by the goal.
func (g *Goal) RequiredBindings() []Binding {
	if g == nil {
		return nil
	}
	return slices.Clone(g.requiredBindings)
}

// Value evaluates the goal value in worldState. An unconfigured value is zero.
func (g *Goal) Value(worldState WorldState) float64 {
	if g == nil || g.value == nil {
		return 0
	}
	return g.value(worldState)
}

// Requirements merges the configured condition keys and typed bindings into a
// single [ConditionSet]. Every requirement contributes a True condition that
// the planner targets.
func (g *Goal) Requirements() ConditionSet {
	if g == nil {
		return nil
	}
	requirements := ConditionSet{}
	for _, condition := range g.requiredConditions {
		requirements[condition] = True
	}
	for _, binding := range g.requiredBindings {
		requirements[binding.String()] = True
	}
	return requirements
}

// SatisfiedBy reports whether worldState meets every goal requirement.
// It compares only the snapshot's current truth map and never evaluates
// Unknown named conditions. Callers with evaluator-backed conditions must
// resolve them before using this snapshot-only helper.
func (g *Goal) SatisfiedBy(worldState WorldState) bool {
	if g == nil || nilvalue.Is(worldState) {
		return false
	}
	return worldState.Conditions().Satisfies(g.Requirements())
}

// NewOutputGoal builds a Goal whose requirement is "an artifact of
// type T exists on the blackboard" — the canonical "produce a
// BlogPost" shape. The supplied template carries Description, requirements,
// and Value; missing Name, output binding, and Value default-fill from T.
//
//	core.NewOutputGoal[BlogPost](core.GoalConfig{Description: "blog post produced"})
func NewOutputGoal[T any](config GoalConfig) *Goal {
	outputType := reflect.TypeFor[T]()
	typeName := TypeNameOf(outputType)

	if config.Name == "" {
		config.Name = "produce_" + typeName
	}
	config.RequiredBindings = append(slices.Clone(config.RequiredBindings), NewBinding[T](DefaultBindingName))
	if config.Value == nil {
		config.Value = FixedScore(defaultGoalValue)
	}
	return NewGoal(config)
}
