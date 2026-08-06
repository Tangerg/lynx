package planning

import (
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

type bindingTarget uint8

const (
	bindingTargetInvalid bindingTarget = iota
	bindingTargetDispatcher
	bindingTargetChild
)

// ChildInputFunc deterministically derives one child Process input from the
// Planning Process input and its latest WorldState. It must perform no I/O and
// must return the same Input for the same arguments. Nil reuses the Planning
// Process input unchanged.
type ChildInputFunc func(agent.Input, WorldState) (agent.Input, error)

// DispatcherBindingConfig binds a predictive Action to the Planning
// Dispatcher. RequiredCapabilities are enforced by Engine before dispatch.
type DispatcherBindingConfig struct {
	Action               Action
	RequiredCapabilities []agent.Capability
}

// ChildBindingConfig binds a predictive Action to one exact child Deployment.
// Every attempt starts a new child Process with a stable Engine-derived
// identity, explicit budget, and attenuated capabilities.
type ChildBindingConfig struct {
	Action       Action
	Deployment   agent.DeploymentRef
	Input        ChildInputFunc
	Budget       agent.Budget
	Capabilities agent.CapabilitySet
}

// ActionBinding is an immutable association between predictive Action
// semantics and exactly one external execution mechanism.
type ActionBinding struct {
	action     Action
	target     bindingTarget
	required   agent.CapabilitySet
	child      agent.ChildSpec
	childInput ChildInputFunc
}

// NewDispatcherBinding binds an Action to a named ActionExecutor supplied to
// NewDispatcher.
func NewDispatcherBinding(config DispatcherBindingConfig) (ActionBinding, error) {
	if !config.Action.Valid() {
		return ActionBinding{}, fmt.Errorf("%w: dispatcher binding Action", ErrInvalidAction)
	}
	required, err := agent.NewCapabilitySet(config.RequiredCapabilities...)
	if err != nil {
		return ActionBinding{}, fmt.Errorf("%w: required capabilities: %w", ErrInvalidAction, err)
	}
	return ActionBinding{action: config.Action, target: bindingTargetDispatcher, required: required}, nil
}

// NewChildBinding binds an Action to an exact child Process specification. The
// parent-scoped ChildKey and derived Input are supplied per attempt.
func NewChildBinding(config ChildBindingConfig) (ActionBinding, error) {
	if !config.Action.Valid() || !config.Deployment.Valid() || !config.Budget.Valid() || !config.Capabilities.Valid() {
		return ActionBinding{}, fmt.Errorf("%w: invalid child binding", ErrInvalidAction)
	}
	return ActionBinding{
		action: config.Action,
		target: bindingTargetChild,
		child: agent.ChildSpec{
			Deployment: config.Deployment, Budget: config.Budget, Capabilities: config.Capabilities,
		},
		childInput: config.Input,
	}, nil
}

// Action returns the immutable predictive Action owned by the binding.
func (binding ActionBinding) Action() Action { return binding.action }

// Valid reports whether the binding has one complete execution target.
func (binding ActionBinding) Valid() bool {
	if !binding.action.Valid() || !binding.required.Valid() {
		return false
	}
	switch binding.target {
	case bindingTargetDispatcher:
		return !binding.child.Deployment.Valid() && !binding.child.Budget.Valid() &&
			binding.childInput == nil
	case bindingTargetChild:
		return len(binding.required.Values()) == 0 && binding.child.Deployment.Valid() &&
			binding.child.Budget.Valid() && binding.child.Capabilities.Valid()
	default:
		return false
	}
}

func (binding ActionBinding) childSpec(key agent.ChildKey, input agent.Input) agent.ChildSpec {
	spec := binding.child
	spec.Key = key
	spec.Input = input
	return spec
}
