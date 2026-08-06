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
	phaseAwaitingForkStarts    phase = "awaiting_fork_starts"
	phaseAwaitingForkWaitOpen  phase = "awaiting_fork_wait_open"
	phaseWaitingFork           phase = "waiting_fork"
	phaseCompleted             phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReady, phaseAwaitingChildStart, phaseAwaitingChildWaitOpen,
		phaseWaitingChild, phaseAwaitingForkStarts, phaseAwaitingForkWaitOpen,
		phaseWaitingFork, phaseCompleted:
		return true
	default:
		return false
	}
}

type executionState struct {
	Phase          phase              `json:"phase"`
	Stage          uint32             `json:"stage"`
	Value          json.RawMessage    `json:"value"`
	SelectedCase   string             `json:"selected_case,omitempty"`
	ChildProcessID *agent.ProcessID   `json:"child_process_id,omitempty"`
	WaitID         *agent.WaitID      `json:"wait_id,omitempty"`
	ForkNext       uint32             `json:"fork_next,omitempty"`
	ForkWindow     []forkChildState   `json:"fork_window,omitempty"`
	ForkOutputs    []*json.RawMessage `json:"fork_outputs,omitempty"`
}

type forkChildState struct {
	Branch    uint32           `json:"branch"`
	ProcessID *agent.ProcessID `json:"process_id,omitempty"`
	Failure   *agent.Failure   `json:"failure,omitempty"`
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
		if state.Stage >= uint32(len(definition.stages)) || !state.noProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildStart:
		if !state.singleChildStage(definition) || state.ChildProcessID != nil || state.WaitID != nil || state.hasForkProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildWaitOpen:
		if !state.singleChildStage(definition) || !hasChild || state.WaitID != nil || state.hasForkProgress() {
			return ErrInvalidExecutionState
		}
	case phaseWaitingChild:
		if !state.singleChildStage(definition) || !hasChild || !hasWait || state.hasForkProgress() {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingForkStarts, phaseAwaitingForkWaitOpen, phaseWaitingFork:
		if state.SelectedCase != "" || state.ChildProcessID != nil {
			return ErrInvalidExecutionState
		}
		if err := state.validateFork(definition); err != nil {
			return ErrInvalidExecutionState
		}
	case phaseCompleted:
		if state.Stage != uint32(len(definition.stages)) || !state.noProgress() {
			return ErrInvalidExecutionState
		}
	}
	return nil
}

func (state executionState) singleChildStage(definition *Definition) bool {
	if state.Stage >= uint32(len(definition.stages)) {
		return false
	}
	stage := definition.stages[state.Stage]
	switch stage.kind {
	case StageKindCall:
		return state.SelectedCase == ""
	case StageKindSwitch:
		_, found := stage.switcher.binding(state.SelectedCase)
		return found
	default:
		return false
	}
}

func (state executionState) noProgress() bool {
	return state.SelectedCase == "" && state.ChildProcessID == nil && state.WaitID == nil &&
		state.ForkNext == 0 && state.ForkWindow == nil && state.ForkOutputs == nil
}

func (state executionState) hasForkProgress() bool {
	return state.ForkNext != 0 || state.ForkWindow != nil || state.ForkOutputs != nil
}

func (state executionState) validateFork(definition *Definition) error {
	if state.Stage >= uint32(len(definition.stages)) {
		return ErrInvalidExecutionState
	}
	stage := definition.stages[state.Stage]
	if stage.kind != StageKindFork || !stage.fork.valid() || state.ForkNext == 0 ||
		uint64(state.ForkNext) > uint64(len(stage.fork.branches)) || len(state.ForkWindow) == 0 ||
		uint64(len(state.ForkWindow)) > uint64(stage.fork.concurrency) ||
		len(state.ForkOutputs) != len(stage.fork.branches) {
		return ErrInvalidExecutionState
	}
	windowStart := state.ForkNext - uint32(len(state.ForkWindow))
	resolved := 0
	started := 0
	for offset, child := range state.ForkWindow {
		if child.Branch != windowStart+uint32(offset) {
			return ErrInvalidExecutionState
		}
		hasProcess := child.ProcessID != nil && child.ProcessID.Valid()
		hasFailure := child.Failure != nil && child.Failure.Valid()
		if child.ProcessID != nil && !hasProcess || child.Failure != nil && !hasFailure {
			return ErrInvalidExecutionState
		}
		if hasProcess || hasFailure {
			resolved++
		}
		if hasProcess {
			started++
		}
	}
	if resolved != 0 && resolved != len(state.ForkWindow) {
		return ErrInvalidExecutionState
	}
	for index, output := range state.ForkOutputs {
		if uint32(index) < windowStart {
			if output == nil {
				return ErrInvalidExecutionState
			}
			value, err := agent.ParseOutput(*output)
			if err != nil {
				return ErrInvalidExecutionState
			}
			if err := stage.fork.branchSchema.ValidateOutput(value); err != nil {
				return ErrInvalidExecutionState
			}
		} else if output != nil {
			return ErrInvalidExecutionState
		}
	}
	switch state.Phase {
	case phaseAwaitingForkStarts:
		if state.WaitID != nil || resolved != 0 && started != 0 {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingForkWaitOpen:
		if state.WaitID != nil || resolved != len(state.ForkWindow) || started == 0 {
			return ErrInvalidExecutionState
		}
		for _, child := range state.ForkWindow {
			if child.ProcessID != nil && child.Failure != nil {
				return ErrInvalidExecutionState
			}
		}
	case phaseWaitingFork:
		if state.WaitID == nil || !state.WaitID.Valid() || resolved != len(state.ForkWindow) || started == 0 {
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
