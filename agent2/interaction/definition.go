package interaction

import (
	"encoding/json"
	"fmt"
	"slices"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	executionStateKind          = "interaction"
	executionStateSchemaVersion = 3
)

// DefinitionConfig describes immutable Interaction behavior. MaxModelCalls is
// required because a model-directed loop must have an explicit local stop
// condition in addition to Engine-wide Effect and Step limits.
type DefinitionConfig struct {
	// Name is the stable qualified Definition name.
	Name string

	// Description states the managed behavior for discovery.
	Description string

	// Version is the semantic version of the Definition contract.
	Version string

	// MaxModelCalls bounds model Effects in one Interaction. It must be positive.
	MaxModelCalls uint32

	// Delegates is the frozen model-visible manifest of exact child
	// Deployments. Names must be unique within this slice and must not collide
	// with ordinary Tools bound by the Dispatcher.
	Delegates []Delegate

	// CompletionValidator optionally verifies a proposed final semantic output
	// against the current WorkingContext and accumulated typed Delegate
	// Artifacts. It is a pure Strategy callback whose identity must be covered by
	// the Deployment's ConfigurationDigest. Nil accepts every otherwise valid
	// completion.
	CompletionValidator CompletionValidator
}

// Definition is an immutable managed model/Tool-loop definition. It contains
// no model client or executable Tool; those external capabilities belong to
// the Deployment-bound Dispatcher.
type Definition struct {
	descriptor          agent.Descriptor
	maxModelCalls       uint32
	delegates           []Delegate
	delegateByName      map[string]Delegate
	completionValidator CompletionValidator
}

// NewDefinition validates config and constructs an Interaction Definition.
// A Deployment's ConfigurationDigest must cover the ordered Delegate bindings
// because they affect model behavior and child execution.
func NewDefinition(config DefinitionConfig) (*Definition, error) {
	if config.MaxModelCalls == 0 {
		return nil, fmt.Errorf("%w: MaxModelCalls must be positive", ErrInvalidDefinitionConfig)
	}
	inputSchema, err := agent.SchemaFor[Input]()
	if err != nil {
		return nil, fmt.Errorf("%w: input schema: %w", ErrInvalidDefinitionConfig, err)
	}
	outputSchema, err := agent.SchemaFor[Output]()
	if err != nil {
		return nil, fmt.Errorf("%w: output schema: %w", ErrInvalidDefinitionConfig, err)
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name:         config.Name,
		Description:  config.Description,
		Version:      config.Version,
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: descriptor: %w", ErrInvalidDefinitionConfig, err)
	}
	delegates := slices.Clone(config.Delegates)
	byName := make(map[string]Delegate, len(delegates))
	for index, delegate := range delegates {
		if !delegate.Valid() {
			return nil, fmt.Errorf("%w: Delegates[%d]: %w", ErrInvalidDefinitionConfig, index, ErrInvalidDelegate)
		}
		name := delegate.definition.Name
		if _, duplicate := byName[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Delegate name %q", ErrInvalidDefinitionConfig, name)
		}
		delegate = delegate.clone()
		delegates[index] = delegate
		byName[name] = delegate
	}
	return &Definition{
		descriptor: descriptor, maxModelCalls: config.MaxModelCalls,
		delegates: delegates, delegateByName: byName,
		completionValidator: config.CompletionValidator,
	}, nil
}

// Descriptor returns the immutable model-visible Definition contract.
func (definition *Definition) Descriptor() agent.Descriptor {
	if definition == nil {
		return agent.Descriptor{}
	}
	return definition.descriptor
}

// Start creates a fresh Interaction from validated caller input.
func (definition *Definition) Start(input agent.Input) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	decoded, err := agent.DecodeInput[Input](input)
	if err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidInput, err)
	}
	if err := decoded.Validate(); err != nil {
		return nil, err
	}
	request := &chat.Request{
		Messages: cloneMessages(decoded.Messages),
		Options:  decoded.Options.Clone(),
	}
	return &execution{
		definition: definition,
		state: executionState{
			Phase:   phaseReadyModel,
			Request: request,
		},
	}, nil
}

// Restore recreates an Interaction solely from its opaque, versioned state.
func (definition *Definition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if state.Kind() != executionStateKind || state.SchemaVersion() != executionStateSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported kind or schema version", ErrInvalidExecutionState)
	}
	var decoded executionState
	if err := decodeStrict(state.Payload(), &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidExecutionState, err)
	}
	if err := decoded.Validate(definition); err != nil {
		return nil, err
	}
	return &execution{definition: definition, state: decoded}, nil
}

func (definition *Definition) valid() bool {
	if definition == nil || !definition.descriptor.Valid() || definition.maxModelCalls == 0 ||
		len(definition.delegates) != len(definition.delegateByName) {
		return false
	}
	for _, delegate := range definition.delegates {
		if !delegate.Valid() || definition.delegateByName[delegate.definition.Name].deploymentRef != delegate.deploymentRef {
			return false
		}
	}
	return true
}

func (definition *Definition) delegate(name string) (Delegate, bool) {
	if definition == nil {
		return Delegate{}, false
	}
	delegate, found := definition.delegateByName[name]
	return delegate, found && delegate.Valid()
}

func encodeState(state executionState) (agent.ExecutionState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return agent.ExecutionState{}, fmt.Errorf("interaction: encode execution state: %w", err)
	}
	return agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
}
