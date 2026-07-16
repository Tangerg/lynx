package core

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/agent/interaction"
)

// ActionFunc is the user-supplied Action body. The framework keeps In/Out
// concrete all the way through to Execute so users get true compile-time type
// safety — the same compile-time guarantee that reflection-based frameworks
// achieve at runtime; Go gets there with generics.
type ActionFunc[In, Out any] func(ctx context.Context, process *ProcessContext, input In) (Out, error)

// FuncAction adapts a typed Go function to [Action]. Construct one with
// [NewAction].
type FuncAction[In, Out any] struct {
	metadata ActionMetadata
	fn       ActionFunc[In, Out]
}

// Metadata returns a defensive snapshot. ActionMetadata contains maps and
// slices; returning the stored value directly would let a caller mutate the
// planner-visible definition (and the typed executor's input/output bindings)
// after construction through an otherwise innocent metadata read.
func (a *FuncAction[In, Out]) Metadata() ActionMetadata {
	return a.metadata.clone()
}

// Execute is the runtime entry point. It pulls the In value from the
// blackboard (using the bound input variable name + type), invokes the typed
// function, and writes the output back. Retries are NOT handled here — that
// is the runtime's executeAction loop, which understands ActionStatus and
// the explicit RetryPolicy.
func (a *FuncAction[In, Out]) Execute(ctx context.Context, process *ProcessContext) (ActionStatus, error) {
	if process == nil {
		return ActionFailed, errors.New("agent.Action.Execute: process context is nil")
	}
	if process.blackboard == nil {
		return ActionFailed, fmt.Errorf("agent.Action.Execute: action %q cannot run: process context has no blackboard", a.metadata.Name)
	}
	if a.fn == nil {
		return ActionFailed, fmt.Errorf("agent.Action.Execute: action %q cannot run: action function is nil", a.metadata.Name)
	}

	input, err := loadTypedInput[In](process.blackboard, a.metadata.Inputs)
	if err != nil {
		return ActionFailed, fmt.Errorf("agent.Action.Execute: action %q cannot run: %w", a.metadata.Name, err)
	}

	output, err := a.fn(ctx, process, input)
	if err != nil {
		var suspended *interaction.SuspendedError
		if errors.As(err, &suspended) {
			status, suspendErr := process.Suspend(ctx, suspended.Suspension)
			if suspendErr != nil {
				return ActionFailed, suspendErr
			}
			if status == ActionWaiting {
				return ActionWaiting, nil
			}
		}
		return ActionFailed, err
	}

	// A handled suspension parks rather than completes. The returned output is
	// unproduced zero value, so don't bind it. The runtime flips the
	// process to StatusWaiting; on resume the action re-runs and (with
	// the response now in durable Suspension state) takes the resumed path.
	if process.suspended {
		return ActionWaiting, nil
	}

	// ClearWorkingState resets working state (preserving protected entries)
	// before binding the output so only the just-produced value
	// survives. Used for state-machine transitions and looping flows
	// that want a clean slate on success.
	if a.metadata.ClearWorkingState {
		process.blackboard.ClearWorkingState()
	}

	a.writeOutput(process.blackboard, output)
	return ActionSucceeded, nil
}

// loadTypedInput looks up the input binding on the blackboard. When multiple
// bindings exist (a multi-input action), only the first is loaded as the
// generic In — additional inputs must be fetched via [Get] from inside the
// action body. Go generics carry a single type parameter cleanly, so
// multi-input actions inevitably resort to manual lookup.
func loadTypedInput[In any](blackboard Blackboard, inputs []Binding) (In, error) {
	var zero In
	if len(inputs) == 0 {
		return zero, nil
	}

	binding := inputs[0]
	value, ok := blackboard.Lookup(binding.Name, binding.Type)
	if !ok {
		return zero, fmt.Errorf("agent.loadTypedInput: blackboard is missing required input %s", binding)
	}

	typed, ok := value.(In)
	if !ok {
		return zero, fmt.Errorf(
			"agent.loadTypedInput: blackboard value %s has type %T, expected %s",
			binding, value, TypeName[In](),
		)
	}
	return typed, nil
}

// writeOutput stores the produced value on the blackboard. The first declared
// output binding is the canonical name; Bind() is used so the dual-binding
// behavior (default name + type-derived name) kicks in.
func (a *FuncAction[In, Out]) writeOutput(blackboard Blackboard, output Out) {
	if len(a.metadata.Outputs) == 0 {
		blackboard.Bind(output)
		return
	}

	binding := a.metadata.Outputs[0]
	if binding.IsDefault() {
		blackboard.Bind(output)
		return
	}
	blackboard.Store(binding.Name, output)
}

// NewAction constructs a typed function-backed action. The framework derives
// its default input and output bindings from the Go types.
func NewAction[In, Out any](
	name string,
	fn ActionFunc[In, Out],
	config ActionConfig,
) *FuncAction[In, Out] {
	config.applyDefaults()

	inputs := slices.Clone(config.Inputs)
	if len(inputs) == 0 {
		inputs = []Binding{NewBinding[In]("")}
	}

	outputs := slices.Clone(config.Outputs)
	if len(outputs) == 0 {
		outputs = []Binding{NewBinding[Out]("")}
	}

	metadata := ActionMetadata{
		Name:              name,
		Description:       config.Description,
		Inputs:            inputs,
		Outputs:           outputs,
		Repeatable:        config.Repeatable,
		Retry:             config.Retry,
		ToolGroups:        cloneToolGroupRequirements(config.ToolGroups),
		Cost:              config.Cost,
		Value:             config.Value,
		ClearWorkingState: config.ClearWorkingState,
	}
	metadata.Preconditions, metadata.Effects = metadata.computePreconditionsAndEffects(config.Preconditions, config.Effects)

	return &FuncAction[In, Out]{metadata: metadata, fn: fn}
}

// computePreconditionsAndEffects derives the planner state transition: every
// input binding becomes a True precondition,
// every output binding becomes a True effect, and the action_ran_<name>
// condition is toggled to keep non-repeatable actions from looping.
func (m ActionMetadata) computePreconditionsAndEffects(extraPreconditions, extraEffects []string) (ConditionSet, ConditionSet) {
	preconditions := ConditionSet{}
	effects := ConditionSet{}

	for _, key := range extraPreconditions {
		preconditions[key] = True
	}
	for _, input := range m.Inputs {
		preconditions[input.String()] = True
	}

	for _, key := range extraEffects {
		effects[key] = True
	}
	for _, output := range m.Outputs {
		effects[output.String()] = True
	}

	// "Have not run yet" is a precondition; "have run" is an effect. The
	// world-state reader promotes the runtime's stored action-run condition into the
	// world state so the planner can prune already-executed actions.
	if !m.Repeatable {
		preconditions[m.RunCondition()] = False
	}
	effects[m.RunCondition()] = True

	return preconditions, effects
}
