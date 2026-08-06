package workflow

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	agent "github.com/Tangerg/lynx/agent2"
)

const (
	executionStateKind          = "workflow"
	executionStateSchemaVersion = 1
)

// DefinitionConfig contains one immutable managed Workflow behavior. Stages
// execute in declaration order and must have exactly matching adjacent schemas.
type DefinitionConfig struct {
	// Name is the stable qualified Definition name.
	Name string

	// Description states the managed orchestration behavior for discovery.
	Description string

	// Version is the semantic version of the Definition contract.
	Version string

	// Stages is a non-empty ordered sequence of sealed operations.
	Stages []Stage
}

// Definition is an immutable managed Workflow Strategy.
type Definition struct {
	descriptor agent.Descriptor
	stages     []Stage
}

// NewDefinition validates all Stage identities and exact schema connections.
// The caller's Deployment ConfigurationDigest must cover the ordered Stage
// configuration and every pure function or child binding that can affect it.
func NewDefinition(config DefinitionConfig) (*Definition, error) {
	if len(config.Stages) == 0 || uint64(len(config.Stages)) > math.MaxUint32 {
		return nil, fmt.Errorf("%w: Stages must contain 1 to %d entries", ErrInvalidDefinitionConfig, uint64(math.MaxUint32))
	}
	stages := slices.Clone(config.Stages)
	identities := make(map[string]struct{}, len(stages))
	for index, stage := range stages {
		if !stage.Valid() {
			return nil, fmt.Errorf("%w: Stages[%d]: %w", ErrInvalidDefinitionConfig, index, ErrInvalidStage)
		}
		if _, duplicate := identities[stage.id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Stage ID %q", ErrInvalidDefinitionConfig, stage.id)
		}
		identities[stage.id] = struct{}{}
		if index > 0 && !stage.accepts(stages[index-1].outputSchema) {
			return nil, fmt.Errorf(
				"%w: Stage %q input schema does not exactly match Stage %q output schema",
				ErrInvalidDefinitionConfig, stage.id, stages[index-1].id,
			)
		}
	}
	descriptor, err := agent.NewDescriptor(agent.DescriptorConfig{
		Name: config.Name, Description: config.Description, Version: config.Version,
		InputSchema: stages[0].inputSchema, OutputSchema: stages[len(stages)-1].outputSchema,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: descriptor: %w", ErrInvalidDefinitionConfig, err)
	}
	return &Definition{descriptor: descriptor, stages: stages}, nil
}

// Descriptor returns the immutable erased Workflow contract.
func (definition *Definition) Descriptor() agent.Descriptor {
	if definition == nil {
		return agent.Descriptor{}
	}
	return definition.descriptor
}

// Stages returns the immutable ordered Stage sequence.
func (definition *Definition) Stages() []Stage {
	if definition == nil {
		return nil
	}
	return slices.Clone(definition.stages)
}

// Start creates a fresh Workflow from validated caller input.
func (definition *Definition) Start(input agent.Input) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if err := definition.descriptor.ValidateInput(input); err != nil {
		return nil, err
	}
	state := executionState{Phase: phaseReady, Value: input.JSON()}
	return &execution{definition: definition, state: state}, nil
}

// Restore recreates a Workflow solely from its opaque state and this exact
// Definition.
func (definition *Definition) Restore(state agent.ExecutionState) (agent.Execution, error) {
	if !definition.valid() {
		return nil, ErrInvalidDefinitionConfig
	}
	if state.Kind() != executionStateKind || state.SchemaVersion() != executionStateSchemaVersion {
		return nil, fmt.Errorf("%w: unsupported kind or schema version", ErrInvalidExecutionState)
	}
	var decoded executionState
	if err := decodeStrict(state.Payload(), &decoded); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrInvalidExecutionState, err)
	}
	if err := decoded.validate(definition); err != nil {
		return nil, err
	}
	return &execution{definition: definition, state: decoded}, nil
}

func (definition *Definition) valid() bool {
	if definition == nil || !definition.descriptor.Valid() || len(definition.stages) == 0 {
		return false
	}
	for index, stage := range definition.stages {
		if !stage.Valid() || index > 0 && !stage.accepts(definition.stages[index-1].outputSchema) {
			return false
		}
	}
	return definition.descriptor.InputSchema().Valid() && definition.descriptor.OutputSchema().Valid()
}

func encodeExecutionState(state executionState) (agent.ExecutionState, error) {
	payload, err := json.Marshal(state)
	if err != nil {
		return agent.ExecutionState{}, fmt.Errorf("workflow: encode execution state: %w", err)
	}
	return agent.NewExecutionState(executionStateKind, executionStateSchemaVersion, payload)
}
