package interaction

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
)

const (
	executionStateKind          = "interaction"
	executionStateSchemaVersion = 8
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
func (d *Definition) Descriptor() agent.Descriptor {
	if d == nil {
		return agent.Descriptor{}
	}
	return d.descriptor
}

// Start creates a fresh Interaction from validated caller input.
func (d *Definition) Start(input agent.Input) (agent.Execution, error) {
	if !d.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	decoded, err := input.Decode[Input]()
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
		definition: d,
		state: executionState{
			Phase:          phaseReadyModel,
			WorkingContext: request,
		},
	}, nil
}

// Restore recreates an Interaction solely from its opaque, versioned state.
func (d *Definition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if !d.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if state.Kind() != executionStateKind || state.SchemaVersion() != executionStateSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported kind or schema version", ErrInvalidExecutionState)
	}
	var decoded executionState
	if err := jsonv2.Unmarshal(state.Payload(), &decoded, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("%w: decode: %w", ErrInvalidExecutionState, err)
	}
	if err := decoded.Validate(d); err != nil {
		return nil, err
	}
	return &execution{definition: d, state: decoded}, nil
}

func (d *Definition) valid() bool {
	if d == nil || !d.descriptor.Valid() || d.maxModelCalls == 0 ||
		len(d.delegates) != len(d.delegateByName) {
		return false
	}
	for _, delegate := range d.delegates {
		if !delegate.Valid() || d.delegateByName[delegate.definition.Name].deploymentRef != delegate.deploymentRef {
			return false
		}
	}
	return true
}

func (d *Definition) delegate(name string) (Delegate, bool) {
	if d == nil {
		return Delegate{}, false
	}
	delegate, found := d.delegateByName[name]
	return delegate, found && delegate.Valid()
}

func encodeState(state executionState) (agent.ExecutionState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return agent.ExecutionState{}, fmt.Errorf("interaction: encode execution state: %w", err)
	}
	return agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
}
