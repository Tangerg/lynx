package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

const maxStageIDBytes = 128

// StageKind identifies one sealed Workflow operation. It is descriptive; new
// behavior cannot be injected by implementing an interface.
type StageKind uint8

const (
	StageKindInvalid StageKind = iota
	StageKindTransform
	StageKindCall
)

func (kind StageKind) String() string {
	switch kind {
	case StageKindTransform:
		return "transform"
	case StageKindCall:
		return "call"
	default:
		return "invalid"
	}
}

// TransformFunc is a bounded, deterministic, side-effect-free value
// transformation. It must not perform I/O, read clock or random state, mutate
// shared state, or start goroutines.
type TransformFunc[I, O any] func(I) (O, error)

type transformStage func(json.RawMessage) (json.RawMessage, error)

type callStage struct {
	deployment   agent.DeploymentRef
	budget       agent.Budget
	capabilities agent.CapabilitySet
}

// Stage is an immutable operation in one Workflow Definition. Values can only
// be constructed by this package, keeping the execution algebra closed.
type Stage struct {
	id           string
	kind         StageKind
	inputSchema  agent.Schema
	outputSchema agent.Schema
	transform    transformStage
	call         callStage
}

// CallConfig declares one exact child Deployment and its non-renewable
// Framework resource allocation.
type CallConfig struct {
	// ID is unique within the Workflow and remains stable across restoration.
	ID string

	// Deployment is the exact child behavior binding. The Stage retains only
	// its immutable DeploymentRef and Descriptor schemas.
	Deployment agent.Deployment

	// Budget is permanently allocated from the parent when the child starts.
	Budget agent.Budget

	// Capabilities is the attenuated authority set granted to the child.
	Capabilities agent.CapabilitySet
}

// Transform constructs one typed pure Stage. JSON schemas derived from I and O
// remain the authoritative erased boundary used by the Workflow Definition.
func Transform[I, O any](id string, transform TransformFunc[I, O]) (Stage, error) {
	if !validStageID(id) || transform == nil {
		return Stage{}, ErrInvalidStage
	}
	inputSchema, err := agent.SchemaFor[I]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: transform %q input schema: %v", ErrInvalidStage, id, err)
	}
	outputSchema, err := agent.SchemaFor[O]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: transform %q output schema: %v", ErrInvalidStage, id, err)
	}
	apply := func(raw json.RawMessage) (json.RawMessage, error) {
		input, err := agent.ParseInput(raw)
		if err != nil {
			return nil, fmt.Errorf("transform %q input: %w", id, err)
		}
		if err := inputSchema.ValidateInput(input); err != nil {
			return nil, fmt.Errorf("transform %q input contract: %w", id, err)
		}
		decoded, err := agent.DecodeInput[I](input)
		if err != nil {
			return nil, fmt.Errorf("transform %q decode input: %w", id, err)
		}
		output, err := transform(decoded)
		if err != nil {
			return nil, fmt.Errorf("transform %q: %w", id, err)
		}
		erased, err := agent.EncodeOutput(output)
		if err != nil {
			return nil, fmt.Errorf("transform %q encode output: %w", id, err)
		}
		if err := outputSchema.ValidateOutput(erased); err != nil {
			return nil, fmt.Errorf("transform %q output contract: %w", id, err)
		}
		return erased.JSON(), nil
	}
	return Stage{
		id: id, kind: StageKindTransform,
		inputSchema: inputSchema, outputSchema: outputSchema, transform: apply,
	}, nil
}

// Call constructs one managed child-Process Stage. No child Process is created
// until the Workflow Execution returns a Framework StartChild Effect.
func Call(config CallConfig) (Stage, error) {
	if !validStageID(config.ID) || !config.Deployment.Valid() ||
		!config.Budget.Valid() || !config.Capabilities.Valid() {
		return Stage{}, ErrInvalidStage
	}
	descriptor := config.Deployment.Descriptor()
	return Stage{
		id: config.ID, kind: StageKindCall,
		inputSchema: descriptor.InputSchema(), outputSchema: descriptor.OutputSchema(),
		call: callStage{
			deployment: config.Deployment.Reference(), budget: config.Budget,
			capabilities: config.Capabilities,
		},
	}, nil
}

// ID returns the stable Definition-local Stage identity.
func (stage Stage) ID() string { return stage.id }

// Kind returns the sealed operation kind.
func (stage Stage) Kind() StageKind { return stage.kind }

// InputSchema returns the authoritative structural input contract.
func (stage Stage) InputSchema() agent.Schema { return stage.inputSchema }

// OutputSchema returns the authoritative structural output contract.
func (stage Stage) OutputSchema() agent.Schema { return stage.outputSchema }

// Valid reports whether the Stage was constructed successfully.
func (stage Stage) Valid() bool {
	if !validStageID(stage.id) || !stage.inputSchema.Valid() || !stage.outputSchema.Valid() {
		return false
	}
	switch stage.kind {
	case StageKindTransform:
		return stage.transform != nil && !stage.call.deployment.Valid() &&
			!stage.call.budget.Valid() && stage.call.capabilities.Valid()
	case StageKindCall:
		return stage.transform == nil && stage.call.deployment.Valid() &&
			stage.call.budget.Valid() && stage.call.capabilities.Valid()
	default:
		return false
	}
}

func (stage Stage) accepts(schema agent.Schema) bool {
	return schema.Valid() && bytes.Equal(stage.inputSchema.JSON(), schema.JSON())
}

func validStageID(value string) bool {
	if len(value) == 0 || len(value) > maxStageIDBytes || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}
