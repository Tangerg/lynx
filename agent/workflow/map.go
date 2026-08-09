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
	inputSchema, err := agent.SchemaFor[[]I]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: Map %q input schema: %w", ErrInvalidStage, config.ID, err)
	}
	itemSchema, err := agent.SchemaFor[I]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: Map %q item schema: %w", ErrInvalidStage, config.ID, err)
	}
	itemOutputSchema, err := agent.SchemaFor[O]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: Map %q item output schema: %w", ErrInvalidStage, config.ID, err)
	}
	outputSchema, err := agent.SchemaFor[[]O]()
	if err != nil {
		return Stage{}, fmt.Errorf("%w: Map %q output schema: %w", ErrInvalidStage, config.ID, err)
	}
	descriptor := config.Deployment.Descriptor()
	if !schemasEqual(itemSchema, descriptor.InputSchema()) ||
		!schemasEqual(itemOutputSchema, descriptor.OutputSchema()) {
		return Stage{}, fmt.Errorf("%w: Map %q child schema mismatch", ErrInvalidStage, config.ID)
	}
	decodeValues := func(raw json.RawMessage) ([]I, error) {
		input, err := agent.ParseInput(raw)
		if err != nil {
			return nil, err
		}
		if err := inputSchema.ValidateInput(input); err != nil {
			return nil, err
		}
		values, err := agent.DecodeInput[[]I](input)
		if err != nil {
			return nil, err
		}
		if uint64(len(values)) > uint64(config.ItemLimit) || uint64(len(values)) > math.MaxUint32 {
			return nil, mapItemLimitExceeded{count: uint64(len(values)), limit: config.ItemLimit}
		}
		return values, nil
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
		if err != nil || start > end || uint64(end) > uint64(len(values)) {
			return nil, errors.Join(ErrInvalidExecutionState, err)
		}
		items := make([]agent.Input, 0, end-start)
		for index := start; index < end; index++ {
			value := values[index]
			item, err := agent.EncodeInput(value)
			if err != nil {
				return nil, fmt.Errorf("Map %q item %d: %w", config.ID, index, err)
			}
			if err := itemSchema.ValidateInput(item); err != nil {
				return nil, fmt.Errorf("Map %q item %d contract: %w", config.ID, index, err)
			}
			items = append(items, item)
		}
		return items, nil
	}
	collect := func(raw []json.RawMessage) (json.RawMessage, error) {
		values := make([]O, len(raw))
		for index, encoded := range raw {
			output, err := agent.ParseOutput(encoded)
			if err != nil {
				return nil, fmt.Errorf("Map %q item %d output: %w", config.ID, index, err)
			}
			if err := itemOutputSchema.ValidateOutput(output); err != nil {
				return nil, fmt.Errorf("Map %q item %d output contract: %w", config.ID, index, err)
			}
			decoded, err := agent.DecodeOutput[O](output)
			if err != nil {
				return nil, fmt.Errorf("Map %q item %d decode output: %w", config.ID, index, err)
			}
			values[index] = decoded
		}
		erased, err := agent.EncodeOutput(values)
		if err != nil {
			return nil, fmt.Errorf("Map %q encode result: %w", config.ID, err)
		}
		if err := outputSchema.ValidateOutput(erased); err != nil {
			return nil, fmt.Errorf("Map %q result contract: %w", config.ID, err)
		}
		return erased.JSON(), nil
	}
	return Stage{
		id: config.ID, kind: stageKindMap,
		inputSchema: inputSchema, outputSchema: outputSchema,
		mapper: mapStage{
			binding: childBinding{
				deploymentRef: config.Deployment.DeploymentRef(), budget: config.Budget,
				capabilities: config.Capabilities,
			},
			windowSize: config.WindowSize, itemLimit: config.ItemLimit,
			itemOutputSchema: itemOutputSchema, count: count,
			windowInputs: windowInputs, collect: collect,
		},
	}, nil
}

type mapItemLimitExceeded struct {
	count uint64
	limit uint32
}

func (failure mapItemLimitExceeded) Error() string {
	return fmt.Sprintf("Map input contains %d items, exceeding limit %d", failure.count, failure.limit)
}
