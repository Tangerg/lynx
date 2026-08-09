package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	agent "github.com/Tangerg/lynx/agent"
)

// MapConfig declares a bounded homogeneous item fan-out. The Stage input is
// []I, each exact child Deployment consumes one I and produces one O, and the
// Stage output is []O in original item order.
type MapConfig[I, O any] struct {
	// ID is unique within the Workflow and remains stable across restoration.
	ID string

	// Deployment is the exact child behavior binding used for every item.
	Deployment agent.Deployment

	// Budget is permanently allocated from the parent for each started item.
	Budget agent.Budget

	// Capabilities is the attenuated authority set granted to each child.
	Capabilities agent.CapabilitySet

	// WindowSize is the positive number of items started and settled as one
	// execution window before the next window begins.
	WindowSize uint32

	// ItemLimit is the positive maximum accepted input length.
	ItemLimit uint32
}

type mapStage struct {
	binding          childBinding
	windowSize       uint32
	itemLimit        uint32
	itemOutputSchema agent.Schema
	count            func(json.RawMessage) (uint32, error)
	windowInputs     func(json.RawMessage, uint32, uint32) ([]agent.Input, error)
	collect          func([]json.RawMessage) (json.RawMessage, error)
}

func (stage mapStage) valid() bool {
	return stage.binding.valid() && stage.windowSize > 0 && stage.itemLimit > 0 &&
		stage.windowSize <= stage.itemLimit && stage.itemOutputSchema.Valid() &&
		stage.count != nil && stage.windowInputs != nil && stage.collect != nil
}

// Map constructs one bounded managed item fan-out Stage. Empty input is valid
// and produces a non-nil empty []O without creating child Processes.
func Map[I, O any](config MapConfig[I, O]) (Stage, error) {
	if !validStageID(config.ID) || !config.Deployment.Valid() ||
		!config.Budget.Valid() || !config.Capabilities.Valid() ||
		config.WindowSize == 0 || config.ItemLimit == 0 || config.WindowSize > config.ItemLimit {
		return Stage{}, ErrInvalidStage
	}
	schemas, err := mapSchemasFor[I, O](config.ID)
	if err != nil {
		return Stage{}, err
	}
	descriptor := config.Deployment.Descriptor()
	if !schemasEqual(schemas.itemInput, descriptor.InputSchema()) ||
		!schemasEqual(schemas.itemOutput, descriptor.OutputSchema()) {
		return Stage{}, fmt.Errorf("%w: Map %q child schema mismatch", ErrInvalidStage, config.ID)
	}
	decodeValues := func(raw json.RawMessage) ([]I, error) {
		return decodeMapValues[I](raw, schemas.input, config.ItemLimit)
	}
	count := func(raw json.RawMessage) (uint32, error) {
		values, err := decodeValues(raw)
		if err != nil {
			return 0, err
		}
		return uint32(len(values)), nil
	}
	windowInputs := func(raw json.RawMessage, start, end uint32) ([]agent.Input, error) {
		values, err := decodeValues(raw)
		if err != nil {
			return nil, errors.Join(ErrInvalidExecutionState, err)
		}
		return encodeMapWindow(config.ID, schemas.itemInput, values, start, end)
	}
	collect := func(raw []json.RawMessage) (json.RawMessage, error) {
		return collectMapOutputs[O](config.ID, schemas.itemOutput, schemas.output, raw)
	}
	return Stage{
		id: config.ID, kind: stageKindMap,
		inputSchema: schemas.input, outputSchema: schemas.output,
		mapper: mapStage{
			binding: childBinding{
				deploymentRef: config.Deployment.DeploymentRef(), budget: config.Budget,
				capabilities: config.Capabilities,
			},
			windowSize: config.WindowSize, itemLimit: config.ItemLimit,
			itemOutputSchema: schemas.itemOutput, count: count,
			windowInputs: windowInputs, collect: collect,
		},
	}, nil
}

type mapSchemas struct {
	input      agent.Schema
	itemInput  agent.Schema
	itemOutput agent.Schema
	output     agent.Schema
}

func mapSchemasFor[I, O any](id string) (mapSchemas, error) {
	input, err := agent.SchemaFor[[]I]()
	if err != nil {
		return mapSchemas{}, fmt.Errorf("%w: Map %q input schema: %w", ErrInvalidStage, id, err)
	}
	itemInput, err := agent.SchemaFor[I]()
	if err != nil {
		return mapSchemas{}, fmt.Errorf("%w: Map %q item schema: %w", ErrInvalidStage, id, err)
	}
	itemOutput, err := agent.SchemaFor[O]()
	if err != nil {
		return mapSchemas{}, fmt.Errorf("%w: Map %q item output schema: %w", ErrInvalidStage, id, err)
	}
	output, err := agent.SchemaFor[[]O]()
	if err != nil {
		return mapSchemas{}, fmt.Errorf("%w: Map %q output schema: %w", ErrInvalidStage, id, err)
	}
	return mapSchemas{input: input, itemInput: itemInput, itemOutput: itemOutput, output: output}, nil
}

func decodeMapValues[I any](raw json.RawMessage, schema agent.Schema, limit uint32) ([]I, error) {
	input, err := agent.ParseInput(raw)
	if err != nil {
		return nil, err
	}
	if err := schema.ValidateInput(input); err != nil {
		return nil, err
	}
	values, err := agent.DecodeInput[[]I](input)
	if err != nil {
		return nil, err
	}
	if uint64(len(values)) > uint64(limit) || uint64(len(values)) > math.MaxUint32 {
		return nil, mapItemLimitExceeded{count: uint64(len(values)), limit: limit}
	}
	return values, nil
}

func encodeMapWindow[I any](
	id string,
	schema agent.Schema,
	values []I,
	start uint32,
	end uint32,
) ([]agent.Input, error) {
	if start > end || uint64(end) > uint64(len(values)) {
		return nil, ErrInvalidExecutionState
	}
	items := make([]agent.Input, 0, end-start)
	for index := start; index < end; index++ {
		item, err := agent.EncodeInput(values[index])
		if err != nil {
			return nil, fmt.Errorf("Map %q item %d: %w", id, index, err)
		}
		if err := schema.ValidateInput(item); err != nil {
			return nil, fmt.Errorf("Map %q item %d contract: %w", id, index, err)
		}
		items = append(items, item)
	}
	return items, nil
}

func collectMapOutputs[O any](
	id string,
	itemSchema agent.Schema,
	outputSchema agent.Schema,
	raw []json.RawMessage,
) (json.RawMessage, error) {
	values, err := decodeFanoutOutputs[O]("Map", id, "item", itemSchema, raw)
	if err != nil {
		return nil, err
	}
	erased, err := agent.EncodeOutput(values)
	if err != nil {
		return nil, fmt.Errorf("Map %q encode result: %w", id, err)
	}
	if err := outputSchema.ValidateOutput(erased); err != nil {
		return nil, fmt.Errorf("Map %q result contract: %w", id, err)
	}
	return erased.JSON(), nil
}

type mapItemLimitExceeded struct {
	count uint64
	limit uint32
}

func (failure mapItemLimitExceeded) Error() string {
	return fmt.Sprintf("Map input contains %d items, exceeding limit %d", failure.count, failure.limit)
}
