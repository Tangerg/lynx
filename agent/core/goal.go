package core

import (
	"reflect"
	"slices"
)

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

	// Tags are short keywords surfaced to model-driven goal selectors
	// (such as routing.ModelRanker) so the model has a richer signal
	// than just Name + Description. Typical: ["coding", "refactor"]
	// or ["sentiment", "review"]. Optional; planner ignores them.
	Tags []string

	// Examples are sample user inputs that should match this goal —
	// few-shot anchors for LLM rankers. Optional; planner ignores
	// them. Typical: ["Refactor this Go file", "Rename the Foo type"].
	Examples []string

	// Tool, when non-nil, makes this goal available to runtime tool adapters.
	// Runtime helpers walk deployed agents and build tools for these goals.
	// Nil means "internal only; not auto-exposed". The framework's
	// reader is on the runtime side (`runtime.Engine.GoalTools` /
	// `runtime.Engine.StandaloneGoalTools`); leaving Tool non-nil without those
	// callers wired is harmless but also means nothing happens — the
	// field exists to drive user-facing fan-out, not to gate planner
	// behavior.
	Tool *GoalTool
}

// Goal is an immutable target state. The planner finds an action sequence
// whose cumulative effects satisfy Preconditions and ranks it using Value.
type Goal struct {
	name          string
	description   string
	preconditions []string
	inputs        []Binding
	value         ScoreFunc
	tags          []string
	examples      []string
	tool          *GoalTool
}

// NewGoal constructs an immutable goal from config.
func NewGoal(config GoalConfig) *Goal {
	return &Goal{
		name:          config.Name,
		description:   config.Description,
		preconditions: slices.Clone(config.Preconditions),
		inputs:        slices.Clone(config.Inputs),
		value:         config.Value,
		tags:          slices.Clone(config.Tags),
		examples:      slices.Clone(config.Examples),
		tool:          config.Tool.clone(),
	}
}

func (t *GoalTool) clone() *GoalTool {
	if t == nil {
		return nil
	}
	cloned := *t
	return &cloned
}

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

// Tags returns model-routing hints.
func (g *Goal) Tags() []string {
	if g == nil {
		return nil
	}
	return slices.Clone(g.tags)
}

// Examples returns few-shot routing examples.
func (g *Goal) Examples() []string {
	if g == nil {
		return nil
	}
	return slices.Clone(g.examples)
}

// Tool returns a defensive copy of the optional tool configuration.
func (g *Goal) Tool() *GoalTool {
	if g == nil {
		return nil
	}
	return g.tool.clone()
}

// Value evaluates the goal value in worldState. An unconfigured value is zero.
func (g *Goal) Value(worldState WorldState) float64 {
	if g == nil || g.value == nil {
		return 0
	}
	return g.value(worldState)
}

// GoalTool carries the metadata `runtime.Engine.GoalTools` and
// `runtime.Engine.StandaloneGoalTools` need to compile a goal into a
// `tools.Tool`. Build via [NewGoalTool] so the input type's
// schema is captured for the LLM tool definition.
type GoalTool struct {
	// Standalone, when true, makes the goal eligible for top-level
	// publishing (no parent process required) — typically MCP server
	// export. When false, only the in-process supervisor variant
	// (parent's LLM tool loop) picks it up.
	Standalone bool

	// Description overrides Goal.Description when surfacing the goal
	// as an externally-facing tool. Useful when the internal
	// description is too implementation-flavored for an LLM caller.
	// Empty falls back to Goal.Description.
	Description string

	inputType reflect.Type
}

// InputType returns the logical input type captured by [NewGoalTool].
func (t *GoalTool) InputType() reflect.Type {
	if t == nil {
		return nil
	}
	return t.inputType
}

// GoalToolConfig configures how a goal is exposed as a tool.
type GoalToolConfig struct {
	Standalone  bool
	Description string
}

// NewGoalTool constructs publication metadata and captures a
// zero-value of In so tooling can derive the tool's JSON schema and
// drive a typed unmarshal at call time without the user passing a
// loose `any` value.
//
// Example:
//
//	core.NewOutputGoal[BlogPost](core.GoalConfig{
//	    Description: "Produce a blog post about a topic",
//	    Tool: core.NewGoalTool[Topic](core.GoalToolConfig{Standalone: true}),
//	})
//
// In is the agent's logical input type (the type the first action
// consumes), NOT the goal's output type — the goal already encodes
// its output type via [NewOutputGoal].
func NewGoalTool[In any](config GoalToolConfig) *GoalTool {
	return &GoalTool{
		Standalone:  config.Standalone,
		Description: config.Description,
		inputType:   reflect.TypeFor[In](),
	}
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
	if g == nil || worldState == nil {
		return false
	}
	state := worldState.Conditions()
	for key, required := range g.Preconditions() {
		if state[key] != required {
			return false
		}
	}
	return true
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
		config.Value = FixedScore(1.0)
	}
	return NewGoal(config)
}
