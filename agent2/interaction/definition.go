package interaction

import (
	"encoding/json"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	executionStateKind          = "interaction"
	executionStateSchemaVersion = 1
)

// Definition is an immutable managed model/tool-loop definition. It contains
// no model client or executable tool; those external capabilities belong to
// the Deployment-bound Dispatcher.
type Definition struct {
	descriptor    agent.Descriptor
	maxModelCalls uint32
}

// NewDefinition validates config and constructs an Interaction Definition.
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
	return &Definition{descriptor: descriptor, maxModelCalls: config.MaxModelCalls}, nil
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
	if definition == nil || !definition.descriptor.Valid() || definition.maxModelCalls == 0 {
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
		maxModelCalls: definition.maxModelCalls,
		state: executionState{
			Phase:   phaseReadyModel,
			Request: request,
		},
	}, nil
}

// Restore recreates an Interaction solely from its opaque, versioned state.
func (definition *Definition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if definition == nil || !definition.descriptor.Valid() || definition.maxModelCalls == 0 {
		return nil, ErrInvalidDefinitionConfig
	}
	if state.Kind() != executionStateKind || state.SchemaVersion() != executionStateSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported kind or schema version", ErrInvalidState)
	}
	var decoded executionState
	if err := decodeStrict(state.Payload(), &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidState, err)
	}
	if err := decoded.Validate(definition.maxModelCalls); err != nil {
		return nil, err
	}
	return &execution{maxModelCalls: definition.maxModelCalls, state: decoded}, nil
}

func encodeState(state executionState) (agent.ExecutionState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return agent.ExecutionState{}, fmt.Errorf("interaction: encode execution state: %w", err)
	}
	return agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
}
