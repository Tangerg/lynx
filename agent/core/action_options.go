package core

import "reflect"

// ActionOption configures an Action at construction time. Functional options
// are the Go-idiomatic way to express what embabel encodes via the Kotlin
// builder DSL — they keep NewAction's signature small while allowing rich
// per-action tuning.
type ActionOption func(*actionConfig)

// actionConfig is the internal shape that NewAction[In,Out] uses to assemble
// ActionMetadata. Each ActionOption mutates fields here; the typed-action
// constructor reads them out.
type actionConfig struct {
	description     string
	pre             []string
	post            []string
	canRerun        bool
	readOnly        bool
	qos             ActionQos
	costStatic      float64
	valueStatic     float64
	costFn          CostFunc
	valueFn         CostFunc
	toolGroups      []ToolGroupRequirement
	trigger         reflect.Type
	outputBinding   string
	inputBinding    string
	inputs          []IoBinding
	outputs         []IoBinding
	clearBlackboard bool
}

// defaultActionConfig is the seed every typed-action constructor starts from.
// Static cost defaults to 1.0 so the planner doesn't accidentally treat real
// work as zero-cost.
func defaultActionConfig() actionConfig {
	return actionConfig{
		canRerun:   false,
		qos:        DefaultActionQos(),
		costStatic: 1.0,
	}
}

// WithDescription attaches human-readable prose surfaced in tracing,
// dashboards, and (when an action gets exposed as a tool) the LLM prompt.
func WithDescription(s string) ActionOption {
	return func(c *actionConfig) { c.description = s }
}

// WithPre adds explicit preconditions on top of the auto-derived ones.
// Useful when an action depends on a named boolean condition rather than a
// type binding (e.g. WithPre("user_authenticated")).
func WithPre(conditions ...string) ActionOption {
	return func(c *actionConfig) { c.pre = append(c.pre, conditions...) }
}

// WithPost mirrors WithPre on the effects side: declare named conditions
// the action establishes when it succeeds.
func WithPost(conditions ...string) ActionOption {
	return func(c *actionConfig) { c.post = append(c.post, conditions...) }
}

// WithCanRerun lifts the default once-per-process restriction.
func WithCanRerun(canRerun bool) ActionOption {
	return func(c *actionConfig) { c.canRerun = canRerun }
}

// WithReadOnly marks an action as side-effect-free; the planner is free to
// reorder or repeat it without worrying about state changes.
func WithReadOnly(readOnly bool) ActionOption {
	return func(c *actionConfig) { c.readOnly = readOnly }
}

// WithQoS overrides the default retry/back-off policy.
func WithQoS(q ActionQos) ActionOption {
	return func(c *actionConfig) { c.qos = q }
}

// WithCost / WithValue set static cost and value scores. For state-dependent
// scoring use WithCostFn / WithValueFn.
func WithCost(v float64) ActionOption  { return func(c *actionConfig) { c.costStatic = v } }
func WithValue(v float64) ActionOption { return func(c *actionConfig) { c.valueStatic = v } }

// WithCostFn installs a state-dependent cost function. When set, it
// overrides the static cost during planning.
func WithCostFn(fn CostFunc) ActionOption {
	return func(c *actionConfig) { c.costFn = fn }
}

// WithValueFn installs a state-dependent value function.
func WithValueFn(fn CostFunc) ActionOption {
	return func(c *actionConfig) { c.valueFn = fn }
}

// WithToolGroups declares the abstract tool requirements (role names) — the
// resolver translates these to concrete tools at execution time.
func WithToolGroups(requirements ...ToolGroupRequirement) ActionOption {
	return func(c *actionConfig) { c.toolGroups = append(c.toolGroups, requirements...) }
}

// WithToolRoles is the shorthand: declare one ToolGroupRequirement per role
// with default ACTION-level termination scope.
func WithToolRoles(roles ...string) ActionOption {
	return func(c *actionConfig) {
		for _, role := range roles {
			c.toolGroups = append(c.toolGroups, ToolGroupRequirement{
				Role:             role,
				TerminationScope: TerminationScopeAction,
			})
		}
	}
}

// WithTrigger registers a "fire when this type appears" auto-action: if the
// trigger type lands on the blackboard, the planner pulls this action in
// regardless of whether it's on the current plan.
func WithTrigger[T any]() ActionOption {
	return func(c *actionConfig) {
		c.trigger = reflect.TypeFor[T]()
	}
}

// WithOutputBinding overrides the default "it" output variable name. Use
// when an action produces multiple distinct artifacts of the same type.
func WithOutputBinding(name string) ActionOption {
	return func(c *actionConfig) { c.outputBinding = name }
}

// WithInputBinding mirrors WithOutputBinding for the single input binding.
func WithInputBinding(name string) ActionOption {
	return func(c *actionConfig) { c.inputBinding = name }
}

// WithInputs replaces the default single-input binding with the supplied
// list. Used when an action needs multiple distinct named inputs (akin to
// embabel's @RequireNameMatch).
func WithInputs(bindings ...IoBinding) ActionOption {
	return func(c *actionConfig) { c.inputs = append(c.inputs, bindings...) }
}

// WithOutputs adds extra output bindings beyond the default one. Rare; most
// actions produce a single canonical artifact.
func WithOutputs(bindings ...IoBinding) ActionOption {
	return func(c *actionConfig) { c.outputs = append(c.outputs, bindings...) }
}

// WithClearBlackboard marks the action as destructive — after it runs, the
// blackboard is wiped (preserving "protected" entries).
func WithClearBlackboard(clear bool) ActionOption {
	return func(c *actionConfig) { c.clearBlackboard = clear }
}
