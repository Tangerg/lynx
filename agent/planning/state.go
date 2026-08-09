package planning

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	agent "github.com/Tangerg/lynx/agent"
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
	Phase                     phase            `json:"phase"`
	Input                     json.RawMessage  `json:"input"`
	WorldState                WorldState       `json:"world_state"`
	PlanningPasses            uint32           `json:"planning_passes"`
	Attempts                  []Attempt        `json:"attempts,omitempty"`
	ExcludedActionNames       []string         `json:"excluded_action_names,omitempty"`
	CurrentActionName         string           `json:"current_action_name,omitempty"`
	ActionConfirmationPending bool             `json:"action_confirmation_pending,omitempty"`
	ChildKey                  *agent.ChildKey  `json:"child_key,omitempty"`
	ChildProcessID            *agent.ProcessID `json:"child_process_id,omitempty"`
	WaitID                    *agent.WaitID    `json:"wait_id,omitempty"`
	FinalOutput               *Output          `json:"final_output,omitempty"`
}

func (state executionState) validate(definition *Definition) error {
	if !state.Phase.valid() || !definition.valid() || !state.WorldState.Valid() {
		return ErrInvalidExecutionState
	}
	input, err := agent.ParseInput(state.Input)
	if err != nil || definition.descriptor.ValidateInput(input) != nil {
		return fmt.Errorf("%w: Input", ErrInvalidExecutionState)
	}
	previouslyExcluded := make(map[string]struct{})
	for index, attempt := range state.Attempts {
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("%w: attempt %d: %w", ErrInvalidExecutionState, index, err)
		}
		if _, found := definition.binding(attempt.ActionName); !found {
			return fmt.Errorf("%w: attempt references unknown Action %q", ErrInvalidExecutionState, attempt.ActionName)
		}
		if _, excluded := previouslyExcluded[attempt.ActionName]; excluded {
			return fmt.Errorf("%w: Action %q was attempted after exclusion", ErrInvalidExecutionState, attempt.ActionName)
		}
		if attempt.Status != AttemptSucceeded {
			previouslyExcluded[attempt.ActionName] = struct{}{}
		}
	}
	if uint64(len(state.Attempts)) > uint64(definition.maxActionAttempts) ||
		!validExcludedActionNames(state.ExcludedActionNames, definition) {
		return ErrInvalidExecutionState
	}
	wantExcluded := make([]string, 0, len(state.Attempts))
	for _, attempt := range state.Attempts {
		if attempt.Status != AttemptSucceeded {
			wantExcluded = append(wantExcluded, attempt.ActionName)
		}
	}
	slices.Sort(wantExcluded)
	wantExcluded = slices.Compact(wantExcluded)
	if !slices.Equal(state.ExcludedActionNames, wantExcluded) {
		return fmt.Errorf("%w: excluded Actions do not match attempt facts", ErrInvalidExecutionState)
	}
	if state.CurrentActionName != "" {
		binding, found := definition.binding(state.CurrentActionName)
		if !found {
			return fmt.Errorf("%w: unknown current Action %q", ErrInvalidExecutionState, state.CurrentActionName)
		}
		if slices.Contains(state.ExcludedActionNames, state.CurrentActionName) {
			return fmt.Errorf("%w: current Action is excluded", ErrInvalidExecutionState)
		}
		if state.Phase == phaseAwaitingAction && binding.target != bindingTargetDispatcher ||
			(state.Phase == phaseAwaitingChildStart || state.Phase == phaseAwaitingChildWaitOpen ||
				state.Phase == phaseWaitingChild) && binding.target != bindingTargetChild {
			return fmt.Errorf("%w: current Action does not match the execution phase", ErrInvalidExecutionState)
		}
		if binding.target == bindingTargetChild && state.ChildKey != nil {
			wantKey, err := planningChildKey(state.CurrentActionName, uint32(len(state.Attempts)+1))
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
			state.FinalOutput.WorldState.Key() != state.WorldState.Key() ||
			state.FinalOutput.PlanningPasses != state.PlanningPasses ||
			!slices.Equal(state.FinalOutput.Attempts, state.Attempts) ||
			state.FinalOutput.Outcome == OutcomeAchieved && !definition.goal.SatisfiedBy(state.WorldState) {
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
		if state.ActionConfirmationPending && passes != attempts+1 ||
			!state.ActionConfirmationPending && (attempts == 0 && passes != 0 || attempts > 0 && passes != attempts) {
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
		if state.CurrentActionName != "" || state.ActionConfirmationPending || !noChild || state.PlanningPasses != 0 || len(state.Attempts) != 0 {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingObservation:
		if !noChild || state.ActionConfirmationPending != (state.CurrentActionName != "") {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingAction:
		if state.CurrentActionName == "" || state.ActionConfirmationPending || !noChild {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildStart:
		if state.CurrentActionName == "" || state.ActionConfirmationPending || !hasChildKey || state.ChildProcessID != nil || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseAwaitingChildWaitOpen:
		if state.CurrentActionName == "" || state.ActionConfirmationPending || !hasChildKey || !hasChildID || state.WaitID != nil {
			return ErrInvalidExecutionState
		}
	case phaseWaitingChild:
		if state.CurrentActionName == "" || state.ActionConfirmationPending || !hasChildKey || !hasChildID || !hasWaitID {
			return ErrInvalidExecutionState
		}
	case phaseCompleted:
		if state.CurrentActionName != "" || state.ActionConfirmationPending || !noChild {
			return ErrInvalidExecutionState
		}
	default:
		return ErrInvalidExecutionState
	}
	return nil
}

func validExcludedActionNames(actionNames []string, definition *Definition) bool {
	for index, name := range actionNames {
		if !validName(name) || index > 0 && actionNames[index-1] >= name {
			return false
		}
		if _, found := definition.binding(name); !found {
			return false
		}
	}
	return true
}

func (state *executionState) excludeAction(name string) {
	index, found := slices.BinarySearch(state.ExcludedActionNames, name)
	if found {
		return
	}
	state.ExcludedActionNames = slices.Insert(state.ExcludedActionNames, index, name)
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
