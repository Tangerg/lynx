package planning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	agent "github.com/Tangerg/lynx/agent2"
)

type phase string

const (
	phaseReadyObservation      phase = "ready_observation"
	phaseAwaitingObservation   phase = "awaiting_observation"
	phaseAwaitingAction        phase = "awaiting_action"
	phaseAwaitingChildStart    phase = "awaiting_child_start"
	phaseAwaitingChildWaitOpen phase = "awaiting_child_wait_open"
	phaseWaitingChild          phase = "waiting_child"
	phaseCompleted             phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReadyObservation, phaseAwaitingObservation, phaseAwaitingAction,
		phaseAwaitingChildStart, phaseAwaitingChildWaitOpen, phaseWaitingChild, phaseCompleted:
		return true
	default:
		return false
	}
}

type executionState struct {
	Phase           phase            `json:"phase"`
	Input           json.RawMessage  `json:"input"`
	World           WorldState       `json:"world"`
	PlanningPasses  uint32           `json:"planning_passes"`
	Attempts        []Attempt        `json:"attempts,omitempty"`
	ExcludedActions []string         `json:"excluded_actions,omitempty"`
	CurrentAction   string           `json:"current_action,omitempty"`
	PendingConfirm  bool             `json:"pending_confirm,omitempty"`
	ChildKey        *agent.ChildKey  `json:"child_key,omitempty"`
	ChildProcessID  *agent.ProcessID `json:"child_process_id,omitempty"`
	WaitID          *agent.WaitID    `json:"wait_id,omitempty"`
	FinalOutput     *Output          `json:"final_output,omitempty"`
}

func (state executionState) validate(definition *Definition) error {
	if !state.Phase.valid() || !definition.valid() || !state.World.Valid() {
		return ErrInvalidExecutionState
	}
	input, err := agent.ParseInput(state.Input)
	if err != nil || definition.descriptor.ValidateInput(input) != nil {
		return fmt.Errorf("%w: Input", ErrInvalidExecutionState)
	}
	previouslyExcluded := make(map[string]struct{})
	for index, attempt := range state.Attempts {
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("%w: attempt %d: %v", ErrInvalidExecutionState, index, err)
		}
		if _, found := definition.binding(attempt.Action); !found {
			return fmt.Errorf("%w: attempt references unknown Action %q", ErrInvalidExecutionState, attempt.Action)
		}
		if _, excluded := previouslyExcluded[attempt.Action]; excluded {
			return fmt.Errorf("%w: Action %q was attempted after exclusion", ErrInvalidExecutionState, attempt.Action)
		}
		if attempt.Status != AttemptSucceeded {
			previouslyExcluded[attempt.Action] = struct{}{}
		}
	}
	if uint64(len(state.Attempts)) > uint64(definition.maxActionAttempts) ||
		!validExcludedActions(state.ExcludedActions, definition) {
		return ErrInvalidExecutionState
	}
	wantExcluded := make([]string, 0, len(state.Attempts))
	for _, attempt := range state.Attempts {
		if attempt.Status != AttemptSucceeded {
			wantExcluded = append(wantExcluded, attempt.Action)
		}
	}
	slices.Sort(wantExcluded)
	wantExcluded = slices.Compact(wantExcluded)
	if !slices.Equal(state.ExcludedActions, wantExcluded) {
		return fmt.Errorf("%w: excluded Actions do not match attempt facts", ErrInvalidExecutionState)
	}
	if state.CurrentAction != "" {
		binding, found := definition.binding(state.CurrentAction)
		if !found {
			return fmt.Errorf("%w: unknown current Action %q", ErrInvalidExecutionState, state.CurrentAction)
		}
		if slices.Contains(state.ExcludedActions, state.CurrentAction) {
			return fmt.Errorf("%w: current Action is excluded", ErrInvalidExecutionState)
		}
		if state.Phase == phaseAwaitingAction && binding.target != bindingTargetDispatcher ||
			(state.Phase == phaseAwaitingChildStart || state.Phase == phaseAwaitingChildWaitOpen ||
				state.Phase == phaseWaitingChild) && binding.target != bindingTargetChild {
			return fmt.Errorf("%w: current Action does not match the execution phase", ErrInvalidExecutionState)
		}
		if binding.target == bindingTargetChild && state.ChildKey != nil {
			wantKey, err := planningChildKey(state.CurrentAction, uint32(len(state.Attempts)+1))
			if err != nil || *state.ChildKey != wantKey {
				return fmt.Errorf("%w: child key does not match the Action attempt", ErrInvalidExecutionState)
			}
		}
	}
	if err := state.validateProgress(); err != nil {
		return err
	}
	if err := state.validatePhase(); err != nil {
		return err
	}
	if state.Phase == phaseCompleted {
		if state.FinalOutput == nil || state.FinalOutput.Validate() != nil ||
			state.FinalOutput.WorldState.Key() != state.World.Key() ||
			state.FinalOutput.PlanningPasses != state.PlanningPasses ||
			!slices.Equal(state.FinalOutput.Attempts, state.Attempts) ||
			state.FinalOutput.Outcome == OutcomeAchieved && !definition.goal.SatisfiedBy(state.World) {
			return ErrInvalidExecutionState
		}
	} else if state.FinalOutput != nil {
		return ErrInvalidExecutionState
	}
	return nil
}

func (state executionState) validateProgress() error {
	attempts := uint64(len(state.Attempts))
	passes := uint64(state.PlanningPasses)
	switch state.Phase {
	case phaseReadyObservation:
		if attempts != 0 || passes != 0 {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingObservation:
		if state.PendingConfirm && passes != attempts+1 ||
			!state.PendingConfirm && (attempts == 0 && passes != 0 || attempts > 0 && passes != attempts) {
			return fmt.Errorf("%w: observation phase counters are inconsistent", ErrInvalidExecutionState)
		}
	case phaseAwaitingAction, phaseAwaitingChildStart, phaseAwaitingChildWaitOpen, phaseWaitingChild:
		if passes != attempts+1 {
			return fmt.Errorf("%w: active Action counters are inconsistent", ErrInvalidExecutionState)
		}
	case phaseCompleted:
		if state.FinalOutput == nil {
			return ErrInvalidExecutionState
		}
		switch state.FinalOutput.Outcome {
		case OutcomeAchieved:
			if passes != attempts {
				return fmt.Errorf("%w: achieved counters are inconsistent", ErrInvalidExecutionState)
			}
		case OutcomeUnreachable:
			if attempts != 0 || passes != 1 {
				return ErrInvalidExecutionState
			}
		case OutcomeStuck:
			if attempts == 0 || passes != attempts && passes != attempts+1 {
				return fmt.Errorf("%w: stuck counters are inconsistent", ErrInvalidExecutionState)
			}
		}
	}
	return nil
}

func (state executionState) validatePhase() error {
	hasChildKey := state.ChildKey != nil && state.ChildKey.Valid()
	hasChildID := state.ChildProcessID != nil && state.ChildProcessID.Valid()
	hasWaitID := state.WaitID != nil && state.WaitID.Valid()
	noChild := state.ChildKey == nil && state.ChildProcessID == nil && state.WaitID == nil
	switch state.Phase {
	case phaseReadyObservation:
		if state.CurrentAction != "" || state.PendingConfirm || !noChild || state.PlanningPasses != 0 || len(state.Attempts) != 0 {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingObservation:
		if !noChild || state.PendingConfirm != (state.CurrentAction != "") {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingAction:
		if state.CurrentAction == "" || state.PendingConfirm || !noChild {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildStart:
		if state.CurrentAction == "" || state.PendingConfirm || !hasChildKey || state.ChildProcessID != nil || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildWaitOpen:
		if state.CurrentAction == "" || state.PendingConfirm || !hasChildKey || !hasChildID || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseWaitingChild:
		if state.CurrentAction == "" || state.PendingConfirm || !hasChildKey || !hasChildID || !hasWaitID {
			return ErrInvalidExecutionState
		}
	case phaseCompleted:
		if state.CurrentAction != "" || state.PendingConfirm || !noChild {
			return ErrInvalidExecutionState
		}
	default:
		return ErrInvalidExecutionState
	}
	return nil
}

func validExcludedActions(actions []string, definition *Definition) bool {
	for index, name := range actions {
		if !validName(name) || index > 0 && actions[index-1] >= name {
			return false
		}
		if _, found := definition.binding(name); !found {
			return false
		}
	}
	return true
}

func (state *executionState) exclude(name string) {
	index, found := slices.BinarySearch(state.ExcludedActions, name)
	if found {
		return
	}
	state.ExcludedActions = slices.Insert(state.ExcludedActions, index, name)
}

func (state executionState) input() (agent.Input, error) {
	return agent.ParseInput(bytes.Clone(state.Input))
}

func diagnostic(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.TrimSpace(value)
	if value == "" {
		return "external operation failed"
	}
	if len(value) <= maxDescriptionBytes {
		return value
	}
	value = value[:maxDescriptionBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "external operation failed"
	}
	return value
}

func validDiagnostic(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		len(value) <= maxDescriptionBytes
}
