package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/lynx/agent"
)

func (stage Stage) fanoutCount(raw json.RawMessage) (uint32, error) {
	switch stage.kind {
	case stageKindFork:
		if !stage.fork.valid() {
			return 0, ErrInvalidStage
		}
		return uint32(len(stage.fork.branches)), nil
	case stageKindMap:
		if !stage.mapper.valid() {
			return 0, ErrInvalidStage
		}
		return stage.mapper.count(raw)
	default:
		return 0, ErrInvalidStage
	}
}

func (stage Stage) fanoutWindowSize() uint32 {
	switch stage.kind {
	case stageKindFork:
		return stage.fork.windowSize
	case stageKindMap:
		return stage.mapper.windowSize
	default:
		return 0
	}
}

func (stage Stage) fanoutBinding(index uint32) (childBinding, bool) {
	switch stage.kind {
	case stageKindFork:
		if uint64(index) >= uint64(len(stage.fork.branches)) {
			return childBinding{}, false
		}
		return stage.fork.branches[index].binding, true
	case stageKindMap:
		return stage.mapper.binding, stage.mapper.valid()
	default:
		return childBinding{}, false
	}
}

func (stage Stage) fanoutWindowInputs(
	start uint32,
	end uint32,
	raw json.RawMessage,
) ([]agent.Input, error) {
	switch stage.kind {
	case stageKindFork:
		input, err := agent.ParseInput(raw)
		if err != nil {
			return nil, err
		}
		inputs := make([]agent.Input, end-start)
		for index := range inputs {
			inputs[index] = input
		}
		return inputs, nil
	case stageKindMap:
		return stage.mapper.windowInputs(raw, start, end)
	default:
		return nil, ErrInvalidStage
	}
}

func (stage Stage) fanoutOutputSchema() agent.Schema {
	switch stage.kind {
	case stageKindFork:
		return stage.fork.branchSchema
	case stageKindMap:
		return stage.mapper.itemOutputSchema
	default:
		return agent.Schema{}
	}
}

func (stage Stage) fanoutComplete(outputs []json.RawMessage) (json.RawMessage, error) {
	switch stage.kind {
	case stageKindFork:
		return stage.fork.reduce(outputs)
	case stageKindMap:
		return stage.mapper.collect(outputs)
	default:
		return nil, ErrInvalidStage
	}
}

func (stage Stage) fanoutMemberID(index uint32) (string, bool) {
	switch stage.kind {
	case stageKindFork:
		if uint64(index) >= uint64(len(stage.fork.branches)) {
			return "", false
		}
		return stage.fork.branches[index].id, true
	case stageKindMap:
		return strconv.FormatUint(uint64(index), 10), true
	default:
		return "", false
	}
}

func (stage Stage) fanoutMemberLabel(index uint32) string {
	id, _ := stage.fanoutMemberID(index)
	if stage.kind == stageKindFork {
		return "branch " + id
	}
	return "item " + id
}

func (stage Stage) fanoutFailureCode(suffix string) string {
	return fmt.Sprintf("workflow.%s.%s_%s", stage.kind.String(), stage.fanoutMemberNoun(), suffix)
}

func (stage Stage) fanoutMemberNoun() string {
	if stage.kind == stageKindFork {
		return "branch"
	}
	return "item"
}

type fanoutOutputDecoder struct {
	stageName  string
	stageID    string
	memberName string
	schema     agent.Schema
}

func (decoder fanoutOutputDecoder) decode[T any](encodedOutputs []json.RawMessage) ([]T, error) {
	values := make([]T, len(encodedOutputs))
	for index, encoded := range encodedOutputs {
		output, err := agent.ParseOutput(encoded)
		if err != nil {
			return nil, fmt.Errorf("%s %q %s %d output: %w", decoder.stageName, decoder.stageID, decoder.memberName, index, err)
		}
		if err := decoder.schema.ValidateOutput(output); err != nil {
			return nil, fmt.Errorf("%s %q %s %d output contract: %w", decoder.stageName, decoder.stageID, decoder.memberName, index, err)
		}
		decoded, err := output.Decode[T]()
		if err != nil {
			return nil, fmt.Errorf("%s %q %s %d decode output: %w", decoder.stageName, decoder.stageID, decoder.memberName, index, err)
		}
		values[index] = decoded
	}
	return values, nil
}
