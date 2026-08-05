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
	Name          string
	Description   string
	Preconditions []string
	Inputs        []Binding

	// Value is the planner's per-tick value probe. [NewOutputGoal]
	// fills [FixedScore](1.0) when left nil.
	Value ScoreFunc
}

// Goal is an immutable target state. The planner finds an action sequence
// whose cumulative effects satisfy Preconditions and ranks it using Value.
type Goal struct {
	name          string
	description   string
	preconditions []string
	inputs        []Binding
	value         ScoreFunc
}

// GoalDescriptor is the immutable, non-executable projection of a planning
// target. It deliberately excludes Value: observers can describe a goal without
// invoking planner policy.
type GoalDescriptor struct {
	name          string
	description   string
	preconditions []string
	inputs        []Binding
}

// NewGoal constructs an immutable goal from config.
func NewGoal(config GoalConfig) *Goal {
	return &Goal{
		name:          config.Name,
		description:   config.Description,
		preconditions: slices.Clone(config.Preconditions),
		inputs:        slices.Clone(config.Inputs),
		value:         config.Value,
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
		name:          g.name,
		description:   g.description,
		preconditions: slices.Clone(g.preconditions),
		inputs:        slices.Clone(g.inputs),
	}
}

// Name returns the goal's identity.
func (d GoalDescriptor) Name() string { return d.name }

// Description returns the caller-supplied human-readable purpose.
func (d GoalDescriptor) Description() string { return d.description }

// RequiredConditions returns an independent snapshot of the named requirements.
func (d GoalDescriptor) RequiredConditions() []string {
	return slices.Clone(d.preconditions)
}

// Inputs returns an independent snapshot of the typed requirements.
func (d GoalDescriptor) Inputs() []Binding { return slices.Clone(d.inputs) }

func (g *Goal) validate() error {
	if g == nil {
		return errors.New("goal is nil")
	}
	var problems []error
	for index, condition := range g.preconditions {
		if err := validateConditionKey(condition); err != nil {
			problems = append(problems, fmt.Errorf("precondition %d: %w", index, err))
		}
	}
	for index, binding := range g.inputs {
		if err := binding.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("input binding %d: %w", index, err))
		}
	}
	return errors.Join(problems...)
}

// RequiredConditions returns the explicitly named condition requirements.
func (g *Goal) RequiredConditions() []string {
	if g == nil {
		return nil
	}
	return slices.Clone(g.preconditions)
}

// Inputs returns the typed bindings required by the goal.
func (g *Goal) Inputs() []Binding {
	if g == nil {
		return nil
	}
	return slices.Clone(g.inputs)
}

// Value evaluates the goal value in worldState. An unconfigured value is zero.
func (g *Goal) Value(worldState WorldState) float64 {
	if g == nil || g.value == nil {
		return 0
	}
	return g.value(worldState)
}

// Preconditions merges the configured condition keys and typed inputs into a
// single [ConditionSet]: each
// listed precondition + each typed input contributes a True
// condition the planner targets.
func (g *Goal) Preconditions() ConditionSet {
	if g == nil {
		return nil
	}
	preconditions := ConditionSet{}
	for _, condition := range g.preconditions {
		preconditions[condition] = True
	}
	for _, input := range g.inputs {
		preconditions[input.String()] = True
	}
	return preconditions
}

// SatisfiedBy reports whether worldState meets every goal precondition.
// Used by planners to check whether the goal is already met.
func (g *Goal) SatisfiedBy(worldState WorldState) bool {
	if g == nil || nilvalue.Is(worldState) {
		return false
	}
	return worldState.Conditions().Satisfies(g.Preconditions())
}

// NewOutputGoal builds a Goal whose precondition is "an artifact of
// type T exists on the blackboard" — the canonical "produce a
// BlogPost" shape. The supplied template carries Description / Pre /
// Value; missing Name + Inputs + Value default-fill from T.
//
//	core.NewOutputGoal[BlogPost](core.GoalConfig{Description: "blog post produced"})
func NewOutputGoal[T any](config GoalConfig) *Goal {
	outputType := reflect.TypeFor[T]()
	typeName := TypeNameOf(outputType)

	if config.Name == "" {
		config.Name = "produce_" + typeName
	}
	config.Inputs = append(slices.Clone(config.Inputs), NewBinding[T](DefaultBindingName))
	if config.Value == nil {
		config.Value = FixedScore(defaultGoalValue)
	}
	return NewGoal(config)
}
