package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	agent "github.com/Tangerg/lynx/agent2"
)

type phase string

const (
	phaseReady                 phase = "ready"
	phaseAwaitingChildStart    phase = "awaiting_child_start"
	phaseAwaitingChildWaitOpen phase = "awaiting_child_wait_open"
	phaseWaitingChild          phase = "waiting_child"
	phaseCompleted             phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReady, phaseAwaitingChildStart, phaseAwaitingChildWaitOpen,
		phaseWaitingChild, phaseCompleted:
		return true
	default:
		return false
	}
}

type executionState struct {
	Phase          phase            `json:"phase"`
	Stage          uint32           `json:"stage"`
	Value          json.RawMessage  `json:"value"`
	ChildProcessID *agent.ProcessID `json:"child_process_id,omitempty"`
	WaitID         *agent.WaitID    `json:"wait_id,omitempty"`
}

func (state executionState) validate(definition *Definition) error {
	if !state.Phase.valid() || !definition.valid() || uint64(state.Stage) > uint64(len(definition.stages)) {
		return ErrInvalidExecutionState
	}
	input, err := agent.ParseInput(state.Value)
	if err != nil {
		return fmt.Errorf("%w: Value: %v", ErrInvalidExecutionState, err)
	}
	if state.Stage < uint32(len(definition.stages)) {
		if err := definition.stages[state.Stage].inputSchema.ValidateInput(input); err != nil {
			return fmt.Errorf("%w: Value does not satisfy current Stage: %v", ErrInvalidExecutionState, err)
		}
	} else {
		output, err := agent.ParseOutput(state.Value)
		if err != nil || definition.descriptor.ValidateOutput(output) != nil {
			return fmt.Errorf("%w: final Value", ErrInvalidExecutionState)
		}
	}
	hasChild := state.ChildProcessID != nil && state.ChildProcessID.Valid()
	hasWait := state.WaitID != nil && state.WaitID.Valid()
	switch state.Phase {
	case phaseReady:
		if state.Stage >= uint32(len(definition.stages)) || state.ChildProcessID != nil || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildStart:
		if !state.callStage(definition) || state.ChildProcessID != nil || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildWaitOpen:
		if !state.callStage(definition) || !hasChild || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseWaitingChild:
		if !state.callStage(definition) || !hasChild || !hasWait {
			return ErrInvalidExecutionState
		}
	case phaseCompleted:
		if state.Stage != uint32(len(definition.stages)) || state.ChildProcessID != nil || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	}
	return nil
}

func (state executionState) callStage(definition *Definition) bool {
	return state.Stage < uint32(len(definition.stages)) &&
		definition.stages[state.Stage].kind == StageKindCall
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("JSON contains multiple values")
	}
	return fmt.Errorf("decode trailing JSON value: %w", err)
}
