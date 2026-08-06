package planning

import (
	"context"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

// DispatcherConfig binds side-effect-free observation and the exact set of
// dispatcher-targeted Action executors required by a Definition. Child-bound
// Actions must not appear in ActionExecutors.
type DispatcherConfig struct {
	// Observer supplies each complete WorldState observation.
	Observer Observer
	// ActionExecutors maps dispatcher-bound Action names to exact executors.
	ActionExecutors map[string]ActionExecutor
}

type boundExecutor struct {
	action   Action
	executor ActionExecutor
}

// Dispatcher executes observation and dispatcher Action Effects emitted by one
// Planning Definition. It is immutable after construction and may serve
// Processes concurrently when Observer and ActionExecutors are concurrent-safe.
type Dispatcher struct {
	descriptor agent.Descriptor
	observer   Observer
	executors  map[string]boundExecutor
}

// NewDispatcher validates and freezes the exact external capabilities required
// by definition. Missing, extra, typed-nil, or child-Action executors are
// rejected before Deployment assembly.
func NewDispatcher(definition *Definition, config DispatcherConfig) (*Dispatcher, error) {
	if !definition.valid() || isNilImplementation(config.Observer) {
		return nil, ErrInvalidDispatcherConfig
	}
	executors := make(map[string]boundExecutor)
	for _, binding := range definition.bindings {
		executor, supplied := config.ActionExecutors[binding.action.name]
		switch binding.target {
		case bindingTargetDispatcher:
			if !supplied || isNilImplementation(executor) {
				return nil, fmt.Errorf("%w: missing executor for Action %q", ErrInvalidDispatcherConfig, binding.action.name)
			}
			executors[binding.action.name] = boundExecutor{action: binding.action, executor: executor}
		case bindingTargetChild:
			if supplied {
				return nil, fmt.Errorf("%w: child Action %q cannot have an executor", ErrInvalidDispatcherConfig, binding.action.name)
			}
		default:
			return nil, ErrInvalidDispatcherConfig
		}
	}
	for name, executor := range config.ActionExecutors {
		if !validName(name) || isNilImplementation(executor) {
			return nil, fmt.Errorf("%w: invalid executor %q", ErrInvalidDispatcherConfig, name)
		}
		if _, found := executors[name]; !found {
			return nil, fmt.Errorf("%w: extra executor for Action %q", ErrInvalidDispatcherConfig, name)
		}
	}
	return &Dispatcher{
		descriptor: definition.descriptor, observer: config.Observer, executors: executors,
	}, nil
}

// Dispatch executes one validated Planning protocol operation. Observer errors
// and valid ActionResult failures are definite failed settlements; an
// ActionExecutor error leaves the Effect outcome unknown.
func (dispatcher *Dispatcher) Dispatch(
	ctx context.Context,
	request agent.EffectRequest,
	_ agent.DeltaEmitter,
) (agent.Settlement, error) {
	if dispatcher == nil || !dispatcher.descriptor.Valid() || isNilImplementation(dispatcher.observer) {
		return agent.Settlement{}, ErrInvalidDispatcherConfig
	}
	envelope, err := decodeEffect(request.Effect().Payload())
	if err != nil {
		return agent.Settlement{}, err
	}
	if err := dispatcher.descriptor.ValidateInput(envelope.Input); err != nil {
		return agent.Settlement{}, fmt.Errorf("%w: Effect Input: %w", ErrInvalidProtocol, err)
	}
	switch envelope.Operation {
	case operationObserve:
		return dispatcher.observe(ctx, request.ID(), envelope.Input)
	case operationAction:
		return dispatcher.execute(ctx, request.ID(), envelope.Input, *envelope.Action)
	default:
		return agent.Settlement{}, ErrInvalidProtocol
	}
}

// ReplayPolicy permits same-identity replay only for side-effect-free
// observation. Action Effects may have irreversible external consequences and
// always require explicit resolution after an unknown attempt.
func (*Dispatcher) ReplayPolicy(effect agent.Effect) agent.ReplayPolicy {
	envelope, err := decodeEffect(effect.Payload())
	if err == nil && envelope.Operation == operationObserve {
		return agent.ReplayPolicySameIdentity
	}
	return agent.ReplayPolicyNever
}

func (dispatcher *Dispatcher) observe(
	ctx context.Context,
	effectID agent.EffectID,
	input agent.Input,
) (agent.Settlement, error) {
	request := ObservationRequest{EffectID: effectID, Input: input}
	if err := validateObservationRequest(request); err != nil {
		return agent.Settlement{}, err
	}
	state, observeErr := dispatcher.observer.Observe(ctx, request)
	if observeErr == nil && !state.Valid() {
		return agent.Settlement{}, errors.New("planning: Observer returned an invalid WorldState")
	}
	payload, err := observationSignal(state, observeErr)
	if err != nil {
		return agent.Settlement{}, err
	}
	status := agent.SettlementStatusSucceeded
	if observeErr != nil {
		status = agent.SettlementStatusFailed
	}
	return agent.NewSettlement(effectID, status, payload)
}

func (dispatcher *Dispatcher) execute(
	ctx context.Context,
	effectID agent.EffectID,
	input agent.Input,
	call actionCall,
) (agent.Settlement, error) {
	bound, found := dispatcher.executors[call.Name]
	if !found || bound.action.description != call.Description || !bound.action.Applicable(call.WorldState) {
		return agent.Settlement{}, fmt.Errorf("%w: Action %q does not match frozen binding", ErrInvalidProtocol, call.Name)
	}
	request := ActionRequest{
		EffectID: effectID, Input: input, ActionName: call.Name,
		ActionDescription: call.Description, WorldState: call.WorldState,
	}
	if err := validateActionRequest(request); err != nil {
		return agent.Settlement{}, err
	}
	result, err := bound.executor.Execute(ctx, request)
	if err != nil {
		return agent.Settlement{}, fmt.Errorf("planning: execute Action %q: %w", call.Name, err)
	}
	if !result.Valid() {
		return agent.Settlement{}, fmt.Errorf("planning: Action %q returned an invalid result", call.Name)
	}
	return NewActionSettlement(effectID, result)
}

var _ agent.Dispatcher = (*Dispatcher)(nil)
