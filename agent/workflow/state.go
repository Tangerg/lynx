package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	agent "github.com/Tangerg/lynx/agent"
)

type phase string

const (
	phaseReady                  phase = "ready"
	phaseAwaitingChildStart     phase = "awaiting_child_start"
	phaseAwaitingChildWaitOpen  phase = "awaiting_child_wait_open"
	phaseWaitingChild           phase = "waiting_child"
	phaseAwaitingFanoutStarts   phase = "awaiting_fanout_starts"
	phaseAwaitingFanoutWaitOpen phase = "awaiting_fanout_wait_open"
	phaseWaitingFanout          phase = "waiting_fanout"
	phaseCompleted              phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReady, phaseAwaitingChildStart, phaseAwaitingChildWaitOpen,
		phaseWaitingChild, phaseAwaitingFanoutStarts, phaseAwaitingFanoutWaitOpen,
		phaseWaitingFanout, phaseCompleted:
		return true
	default:
		return false
	}
}

type executionState struct {
	Phase              phase              `json:"phase"`
	StageIndex         uint32             `json:"stage_index"`
	CurrentValue       json.RawMessage    `json:"current_value"`
	SelectedCaseID     string             `json:"selected_case_id,omitempty"`
	ChildProcessID     *agent.ProcessID   `json:"child_process_id,omitempty"`
	WaitID             *agent.WaitID      `json:"wait_id,omitempty"`
	NextFanoutIndex    uint32             `json:"next_fanout_index,omitempty"`
	ActiveFanoutWindow []fanoutChildState `json:"active_fanout_window,omitempty"`
	FanoutOutputs      []*json.RawMessage `json:"fanout_outputs,omitempty"`
	LoopIteration      uint32             `json:"loop_iteration,omitempty"`
}

type fanoutChildState struct {
	FanoutIndex    uint32           `json:"fanout_index"`
	ChildProcessID *agent.ProcessID `json:"child_process_id,omitempty"`
	Failure        *agent.Failure   `json:"failure,omitempty"`
}

func (state executionState) validate(definition *Definition) error {
	if !state.Phase.valid() || !definition.valid() || uint64(state.StageIndex) > uint64(len(definition.stages)) {
		return ErrInvalidExecutionState
	}
	input, err := agent.ParseInput(state.CurrentValue)
	if err != nil {
		return fmt.Errorf("%w: current value: %w", ErrInvalidExecutionState, err)
	}
	if state.StageIndex < uint32(len(definition.stages)) {
		if err := definition.stages[state.StageIndex].inputSchema.ValidateInput(input); err != nil {
			return fmt.Errorf("%w: current value does not satisfy current Stage: %w", ErrInvalidExecutionState, err)
		}
	} else {
		output, err := agent.ParseOutput(state.CurrentValue)
		if err != nil || definition.descriptor.ValidateOutput(output) != nil {
			return fmt.Errorf("%w: final value", ErrInvalidExecutionState)
		}
	}
	hasChild := state.ChildProcessID != nil && state.ChildProcessID.Valid()
	hasWait := state.WaitID != nil && state.WaitID.Valid()
	switch state.Phase {
	case phaseReady:
		if state.StageIndex >= uint32(len(definition.stages)) || !state.noProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildStart:
		if !state.singleChildStage(definition) || state.ChildProcessID != nil || state.WaitID != nil || state.hasFanoutProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildWaitOpen:
		if !state.singleChildStage(definition) || !hasChild || state.WaitID != nil || state.hasFanoutProgress() {
			return ErrInvalidExecutionState
		}
	case phaseWaitingChild:
		if !state.singleChildStage(definition) || !hasChild || !hasWait || state.hasFanoutProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingFanoutStarts, phaseAwaitingFanoutWaitOpen, phaseWaitingFanout:
		if state.SelectedCaseID != "" || state.ChildProcessID != nil || state.LoopIteration != 0 {
			return ErrInvalidExecutionState
		}
		if err := state.validateFanout(definition); err != nil {
			return ErrInvalidExecutionState
		}
	case phaseCompleted:
		if state.StageIndex != uint32(len(definition.stages)) || !state.noProgress() {
			return ErrInvalidExecutionState
		}
	}
	return nil
}

func (state executionState) singleChildStage(definition *Definition) bool {
	if state.StageIndex >= uint32(len(definition.stages)) {
		return false
	}
	stage := definition.stages[state.StageIndex]
	switch stage.kind {
	case stageKindCall:
		return state.SelectedCaseID == "" && state.LoopIteration == 0
	case stageKindSwitch:
		_, found := stage.switcher.binding(state.SelectedCaseID)
		return found && state.LoopIteration == 0
	case stageKindLoop:
		return state.SelectedCaseID == "" && state.LoopIteration > 0 &&
			state.LoopIteration <= stage.loop.maxIterations
	default:
		return false
	}
}

func (state executionState) noProgress() bool {
	return state.SelectedCaseID == "" && state.ChildProcessID == nil && state.WaitID == nil &&
		state.NextFanoutIndex == 0 && state.ActiveFanoutWindow == nil && state.FanoutOutputs == nil &&
		state.LoopIteration == 0
}

func (state executionState) hasFanoutProgress() bool {
	return state.NextFanoutIndex != 0 || state.ActiveFanoutWindow != nil || state.FanoutOutputs != nil
}

func (state executionState) validateFanout(definition *Definition) error {
	if state.StageIndex >= uint32(len(definition.stages)) {
		return ErrInvalidExecutionState
	}
	stage := definition.stages[state.StageIndex]
	count, err := stage.fanoutCount(state.CurrentValue)
	if err != nil || count == 0 || state.NextFanoutIndex == 0 || state.NextFanoutIndex > count ||
		len(state.ActiveFanoutWindow) == 0 || uint64(len(state.ActiveFanoutWindow)) > uint64(stage.fanoutWindowSize()) ||
		uint64(len(state.FanoutOutputs)) != uint64(count) {
		return ErrInvalidExecutionState
	}
	windowStart := state.NextFanoutIndex - uint32(len(state.ActiveFanoutWindow))
	resolved := 0
	started := 0
	for offset, child := range state.ActiveFanoutWindow {
		if child.FanoutIndex != windowStart+uint32(offset) {
			return ErrInvalidExecutionState
		}
		hasProcess := child.ChildProcessID != nil && child.ChildProcessID.Valid()
		hasFailure := child.Failure != nil && child.Failure.Valid()
		if child.ChildProcessID != nil && !hasProcess || child.Failure != nil && !hasFailure {
			return ErrInvalidExecutionState
		}
		if hasProcess || hasFailure {
			resolved++
		}
		if hasProcess {
			started++
		}
	}
	if resolved != 0 && resolved != len(state.ActiveFanoutWindow) {
		return ErrInvalidExecutionState
	}
	for index, output := range state.FanoutOutputs {
		if uint32(index) < windowStart {
			if output == nil {
				return ErrInvalidExecutionState
			}
			value, err := agent.ParseOutput(*output)
			if err != nil {
				return ErrInvalidExecutionState
			}
			if err := stage.fanoutOutputSchema().ValidateOutput(value); err != nil {
				return ErrInvalidExecutionState
			}
		} else if output != nil {
			return ErrInvalidExecutionState
		}
	}
	switch state.Phase {
	case phaseAwaitingFanoutStarts:
		if state.WaitID != nil || resolved != 0 && started != 0 {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingFanoutWaitOpen:
		if state.WaitID != nil || resolved != len(state.ActiveFanoutWindow) || started == 0 {
			return ErrInvalidExecutionState
		}
		for _, child := range state.ActiveFanoutWindow {
			if child.ChildProcessID != nil && child.Failure != nil {
				return ErrInvalidExecutionState
			}
		}
	case phaseWaitingFanout:
		if state.WaitID == nil || !state.WaitID.Valid() || resolved != len(state.ActiveFanoutWindow) || started == 0 {
			return ErrInvalidExecutionState
		}
	default:
		return ErrInvalidExecutionState
	}
	return nil
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
