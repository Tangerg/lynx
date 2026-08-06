package workflow

import (
	"encoding/json"
	"fmt"
	"strconv"

	agent "github.com/Tangerg/lynx/agent2"
)

func (stage Stage) fanoutCount(raw json.RawMessage) (uint32, error) {
	switch stage.kind {
	case StageKindFork:
		if !stage.fork.valid() {
			return 0, ErrInvalidStage
		}
		return uint32(len(stage.fork.branches)), nil
	case StageKindMap:
		if !stage.mapper.valid() {
			return 0, ErrInvalidStage
		}
		return stage.mapper.count(raw)
	default:
		return 0, ErrInvalidStage
	}
}

func (stage Stage) fanoutConcurrency() uint32 {
	switch stage.kind {
	case StageKindFork:
		return stage.fork.concurrency
	case StageKindMap:
		return stage.mapper.concurrency
	default:
		return 0
	}
}

func (stage Stage) fanoutBinding(index uint32) (childBinding, bool) {
	switch stage.kind {
	case StageKindFork:
		if uint64(index) >= uint64(len(stage.fork.branches)) {
			return childBinding{}, false
		}
		return stage.fork.branches[index].binding, true
	case StageKindMap:
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
	case StageKindFork:
		input, err := agent.ParseInput(raw)
		if err != nil {
			return nil, err
		}
		inputs := make([]agent.Input, end-start)
		for index := range inputs {
			inputs[index] = input
		}
		return inputs, nil
	case StageKindMap:
		return stage.mapper.windowInputs(raw, start, end)
	default:
		return nil, ErrInvalidStage
	}
}

func (stage Stage) fanoutOutputSchema() agent.Schema {
	switch stage.kind {
	case StageKindFork:
		return stage.fork.branchSchema
	case StageKindMap:
		return stage.mapper.itemOutputSchema
	default:
		return agent.Schema{}
	}
}

func (stage Stage) fanoutComplete(outputs []json.RawMessage) (json.RawMessage, error) {
	switch stage.kind {
	case StageKindFork:
		return stage.fork.reduce(outputs)
	case StageKindMap:
		return stage.mapper.collect(outputs)
	default:
		return nil, ErrInvalidStage
	}
}

func (stage Stage) fanoutMemberID(index uint32) (string, bool) {
	switch stage.kind {
	case StageKindFork:
		if uint64(index) >= uint64(len(stage.fork.branches)) {
			return "", false
		}
		return stage.fork.branches[index].id, true
	case StageKindMap:
		return strconv.FormatUint(uint64(index), 10), true
	default:
		return "", false
	}
}

func (stage Stage) fanoutMemberLabel(index uint32) string {
	id, _ := stage.fanoutMemberID(index)
	if stage.kind == StageKindFork {
		return "branch " + id
	}
	return "item " + id
}

func (stage Stage) fanoutFailureCode(suffix string) string {
	return fmt.Sprintf("workflow.%s.%s_%s", stage.kind.String(), stage.fanoutMemberNoun(), suffix)
}

func (stage Stage) fanoutMemberNoun() string {
	if stage.kind == StageKindFork {
		return "branch"
	}
	return "item"
}
