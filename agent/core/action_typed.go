package core

import (
	"context"
	"fmt"
)

// TypedActionFunc is the user-supplied Action body. The framework keeps In/Out
// concrete all the way through to Execute so users get true compile-time type
// safety — embabel achieves the same via Kotlin reflection on JVM signatures,
// which is rich enough; Go gets there with generics.
type TypedActionFunc[In, Out any] func(ctx context.Context, pc *ProcessContext, input In) (Out, error)

// typedAction is the concrete implementation backing NewAction[In,Out]. It
// carries the typed function plus the metadata derived at construction time.
// Unexported because users only ever see the [Action] interface returned
// from [NewAction].
type typedAction[In, Out any] struct {
	metadata ActionMetadata
	fn       TypedActionFunc[In, Out]
}

func (a *typedAction[In, Out]) Metadata() ActionMetadata { return a.metadata }

// Execute is the runtime entry point. It pulls the In value from the
// blackboard (using the bound input variable name + type), invokes the typed
// function, and writes the output back. Retries are NOT handled here — that
// is the runtime's executeAction loop, which understands ActionStatus and
// the QoS policy.
func (a *typedAction[In, Out]) Execute(ctx context.Context, pc *ProcessContext) ActionStatus {
	if pc == nil {
		return ActionFailed
	}
	if pc.Blackboard == nil {
		pc.recordError(fmt.Errorf("action %q cannot run: process context has no blackboard", a.metadata.Name))
		return ActionFailed
	}
	if a.fn == nil {
		pc.recordError(fmt.Errorf("action %q cannot run: action function is nil", a.metadata.Name))
		return ActionFailed
	}

	input, err := loadTypedInput[In](pc.Blackboard, a.metadata.Inputs)
	if err != nil {
		pc.recordError(fmt.Errorf("action %q cannot run: %w", a.metadata.Name, err))
		return ActionFailed
	}

	output, err := a.fn(ctx, pc, input)
	if err != nil {
		pc.recordError(err)
		return ActionFailed
	}

	// HITL: if the fn parked an awaitable via pc.AwaitInput, it
	// suspends rather than completes — the returned output is the
	// unproduced zero value, so don't bind it. The runtime flips the
	// process to StatusWaiting; on resume the action re-runs and (with
	// the response now on the blackboard) takes the non-await path.
	if pc.InputAwaited() {
		return ActionWaiting
	}

	// Mirror embabel's MultiTransformationAction: when the action is
	// flagged ClearBlackboard, wipe (preserving Protected entries)
	// before binding the output so only the just-produced value
	// survives. Used for state-machine transitions and looping flows
	// that want a clean slate on success.
	if a.metadata.ClearBlackboard {
		pc.Blackboard.Clear()
	}

	a.writeOutput(pc.Blackboard, output)
	return ActionSucceeded
}

// loadTypedInput looks up the input binding on the blackboard. When multiple
// bindings exist (a multi-input action), only the first is loaded as the
// generic In — additional inputs must be fetched via Get[T] from inside the
// action body. Go generics carry a single type parameter cleanly, so
// multi-input actions inevitably resort to manual lookup.
func loadTypedInput[In any](bb Blackboard, inputs []IOBinding) (In, error) {
	var zero In
	if len(inputs) == 0 {
		return zero, nil
	}

	binding := inputs[0]
	value, ok := bb.Lookup(binding.Name, binding.Type)
	if !ok {
		return zero, fmt.Errorf("blackboard is missing required input %s", binding)
	}

	typed, ok := value.(In)
	if !ok {
		return zero, fmt.Errorf(
			"blackboard value %s has type %T, expected %s",
			binding, value, TypeName[In](),
		)
	}
	return typed, nil
}

// writeOutput stores the produced value on the blackboard. The first declared
// output binding is the canonical name; Bind() is used so the dual-binding
// behavior (default name + type-derived name) kicks in.
func (a *typedAction[In, Out]) writeOutput(bb Blackboard, output Out) {
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
// they just write functions. Pass [ActionConfig]{} when defaults suffice.
func NewAction[In, Out any](
	name string,
	fn TypedActionFunc[In, Out],
	config ActionConfig,
) Action {
	config.ApplyDefaults()

	inputs := config.Inputs
	if len(inputs) == 0 {
		inputs = []IOBinding{NewIOBinding[In](resolveBindingName(config.InputBinding))}
	}

	outputs := config.Outputs
	if len(outputs) == 0 {
		outputs = []IOBinding{NewIOBinding[Out](resolveBindingName(config.OutputBinding))}
	}

	meta := ActionMetadata{
		Name:            name,
		Description:     config.Description,
		Inputs:          inputs,
		Outputs:         outputs,
		CanRerun:        config.CanRerun,
		QoS:             config.QoS,
		ToolGroups:      config.ToolGroups,
		ToolLoop:        config.ToolLoop,
		Cost:            config.Cost,
		Value:           config.Value,
		OutputBinding:   config.OutputBinding,
		ClearBlackboard: config.ClearBlackboard,
	}
	meta.Preconditions, meta.Effects = computePreconditionsAndEffects(meta, config.Pre, config.Post)

	return &typedAction[In, Out]{metadata: meta, fn: fn}
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
func computePreconditionsAndEffects(meta ActionMetadata, extraPre, extraPost []string) (Effects, Effects) {
	pre := Effects{}
	eff := Effects{}

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
		pre[meta.EffectiveRunKey()] = False
	}
	eff[meta.EffectiveRunKey()] = True

	return pre, eff
}
