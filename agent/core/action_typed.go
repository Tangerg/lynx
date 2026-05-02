package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// TypedActionFunc is the user-supplied Action body. The framework keeps In/Out
// concrete all the way through to Execute so users get true compile-time type
// safety — embabel achieves the same via Kotlin reflection on JVM signatures,
// which is rich enough; Go gets there with generics.
type TypedActionFunc[In, Out any] func(ctx context.Context, pc *ProcessContext, input In) (Out, error)

// TypedAction is the concrete implementation backing NewAction[In,Out]. It
// carries the typed function plus the metadata derived at construction time.
type TypedAction[In, Out any] struct {
	metadata ActionMetadata
	fn       TypedActionFunc[In, Out]
}

func (a *TypedAction[In, Out]) Metadata() ActionMetadata { return a.metadata }

// Execute is the runtime entry point. It pulls the In value from the
// blackboard (using the bound input variable name + type), invokes the typed
// function, and writes the output back. Retries are NOT handled here — that
// is the runtime's executeAction loop, which understands ActionStatus and
// the QoS policy.
func (a *TypedAction[In, Out]) Execute(ctx context.Context, pc *ProcessContext) ActionStatus {
	if pc == nil {
		return ActionFailed
	}
	if pc.Blackboard == nil {
		pc.recordError(errors.New("typed action requires a non-nil Blackboard on ProcessContext"))
		return ActionFailed
	}

	input, ok := loadTypedInput[In](pc.Blackboard, a.metadata.Inputs)
	if !ok {
		pc.recordError(fmt.Errorf(
			"action %q: required input not on blackboard (binding=%s)",
			a.metadata.Name, formatBindings(a.metadata.Inputs),
		))
		return ActionFailed
	}

	output, err := a.fn(ctx, pc, input)
	if err != nil {
		pc.recordError(err)
		return ActionFailed
	}

	a.writeOutput(pc.Blackboard, output)
	return ActionSucceeded
}

// loadTypedInput looks up the input binding on the blackboard. When multiple
// bindings exist (a multi-input action), only the first is loaded as the
// generic In — additional inputs must be fetched via Get[T] from inside the
// action body. Go generics carry a single type parameter cleanly, so
// multi-input actions inevitably resort to manual lookup.
func loadTypedInput[In any](bb Blackboard, inputs []IoBinding) (In, bool) {
	var zero In
	if len(inputs) == 0 {
		return zero, true
	}

	binding := inputs[0]
	value, ok := bb.GetValue(binding.Name, binding.Type)
	if !ok {
		return zero, false
	}

	typed, ok := value.(In)
	if !ok {
		return zero, false
	}
	return typed, true
}

// writeOutput stores the produced value on the blackboard. The first declared
// output binding is the canonical name; Bind() is used so the dual-binding
// behavior (default name + type-derived name) kicks in.
func (a *TypedAction[In, Out]) writeOutput(bb Blackboard, output Out) {
	if len(a.metadata.Outputs) == 0 {
		bb.Bind(output)
		return
	}

	binding := a.metadata.Outputs[0]
	if binding.IsDefault() {
		bb.Bind(output)
		return
	}
	bb.Set(binding.Name, output)
}

// NewAction is the type-safe action constructor. The framework derives input
// and output bindings via reflection on T's type names, then wraps the typed
// fn in an Action interface. Most users never see the Action interface —
// they just write functions.
func NewAction[In, Out any](
	name string,
	fn TypedActionFunc[In, Out],
	opts ...ActionOption,
) Action {
	cfg := defaultActionConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	inputs := cfg.inputs
	if len(inputs) == 0 {
		inputs = []IoBinding{NewIoBinding[In](resolveBindingName(cfg.inputBinding))}
	}

	outputs := cfg.outputs
	if len(outputs) == 0 {
		outputs = []IoBinding{NewIoBinding[Out](resolveBindingName(cfg.outputBinding))}
	}

	meta := ActionMetadata{
		Name:            name,
		Description:     cfg.description,
		Inputs:          inputs,
		Outputs:         outputs,
		CanRerun:        cfg.canRerun,
		ReadOnly:        cfg.readOnly,
		QoS:             cfg.qos,
		ToolGroups:      cfg.toolGroups,
		CostFn:          cfg.costFn,
		ValueFn:         cfg.valueFn,
		CostStatic:      cfg.costStatic,
		ValueStatic:     cfg.valueStatic,
		Trigger:         cfg.trigger,
		OutputBinding:   cfg.outputBinding,
		ClearBlackboard: cfg.clearBlackboard,
	}
	meta.Preconditions, meta.Effects = computePreconditionsAndEffects(meta, cfg.pre, cfg.post)

	return &TypedAction[In, Out]{metadata: meta, fn: fn}
}

// resolveBindingName fills in DefaultBindingName when the caller didn't
// provide an explicit override.
func resolveBindingName(name string) string {
	if name == "" {
		return DefaultBindingName
	}
	return name
}

// computePreconditionsAndEffects mirrors AbstractAction.preconditions /
// .effects in embabel: every input binding becomes a True precondition,
// every output binding becomes a True effect, and the hasRun_<name>
// condition is toggled to keep canRerun=false actions from looping.
func computePreconditionsAndEffects(meta ActionMetadata, extraPre, extraPost []string) (EffectSpec, EffectSpec) {
	pre := EffectSpec{}
	eff := EffectSpec{}

	for _, key := range extraPre {
		pre[key] = True
	}
	for _, in := range meta.Inputs {
		pre[in.String()] = True
	}

	for _, key := range extraPost {
		eff[key] = True
	}
	for _, out := range meta.Outputs {
		eff[out.String()] = True
	}

	// "Have not run yet" is a precondition; "have run" is an effect. The
	// determiner promotes the runtime's stored hasRun condition into the
	// world state so the planner can prune already-executed actions.
	if !meta.CanRerun {
		pre[HasRunKey(meta.Name)] = False
	}
	eff[HasRunKey(meta.Name)] = True

	return pre, eff
}

// formatBindings renders a slice of bindings for inclusion in error
// messages — small helper kept here to keep Execute's error path tidy.
func formatBindings(bindings []IoBinding) string {
	if len(bindings) == 0 {
		return "<none>"
	}
	formatted := make([]string, 0, len(bindings))
	for _, b := range bindings {
		formatted = append(formatted, b.String())
	}
	return strings.Join(formatted, ", ")
}

