package interaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

type phase string

const (
	phaseReadyModel     phase = "ready_model"
	phaseAwaitingModel  phase = "awaiting_model"
	phaseAwaitingTools  phase = "awaiting_tools"
	phaseAwaitingWaitID phase = "awaiting_wait_id"
	phaseWaitingInput   phase = "waiting_input"
	phaseCompleted      phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReadyModel, phaseAwaitingModel, phaseAwaitingTools,
		phaseAwaitingWaitID, phaseWaitingInput, phaseCompleted:
		return true
	default:
		return false
	}
}

// executionState is the complete Strategy-owned recovery state. Request is the
// self-sufficient WorkingContext for the next model call. PendingResponse is
// present only while a model-requested tool batch is being settled.
type executionState struct {
	Phase           phase           `json:"phase"`
	Request         *chat.Request   `json:"request"`
	ModelCalls      uint32          `json:"model_calls"`
	PendingResponse *chat.Response  `json:"pending_response,omitempty"`
	ToolCheckpoint  *toolCheckpoint `json:"tool_checkpoint,omitempty"`
	WaitID          *agent.WaitID   `json:"wait_id,omitempty"`
	FinalOutput     *Output         `json:"final_output,omitempty"`
}

func (state executionState) Validate(maxModelCalls uint32) error {
	if !state.Phase.valid() {
		return fmt.Errorf("%w: unknown phase %q", ErrInvalidState, state.Phase)
	}
	if state.Request == nil {
		return fmt.Errorf("%w: request is required", ErrInvalidState)
	}
	if err := state.Request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidState, err)
	}
	if len(state.Request.Tools) != 0 {
		return fmt.Errorf("%w: executable tool definitions do not belong in WorkingContext", ErrInvalidState)
	}
	if state.ModelCalls > maxModelCalls {
		return fmt.Errorf("%w: model call count exceeds configured limit", ErrInvalidState)
	}
	switch state.Phase {
	case phaseReadyModel:
		if state.PendingResponse != nil || state.ToolCheckpoint != nil || state.WaitID != nil || state.FinalOutput != nil {
			return fmt.Errorf("%w: ready_model has inconsistent pending response or limit", ErrInvalidState)
		}
	case phaseAwaitingModel:
		if state.PendingResponse != nil || state.ToolCheckpoint != nil || state.WaitID != nil || state.FinalOutput != nil || state.ModelCalls == 0 {
			return fmt.Errorf("%w: awaiting_model has inconsistent pending response or limit", ErrInvalidState)
		}
	case phaseAwaitingTools:
		if state.PendingResponse == nil || state.WaitID != nil || state.FinalOutput != nil || state.ModelCalls == 0 {
			return fmt.Errorf("%w: awaiting_tools requires a model response", ErrInvalidState)
		}
		if err := state.PendingResponse.Validate(); err != nil {
			return fmt.Errorf("%w: pending response: %w", ErrInvalidState, err)
		}
		calls, _, err := responseToolCalls(state.PendingResponse)
		if err != nil || len(calls) == 0 {
			return fmt.Errorf("%w: pending response has no unambiguous tool calls", ErrInvalidState)
		}
		if state.ToolCheckpoint != nil {
			if err := state.ToolCheckpoint.validate(calls); err != nil {
				return fmt.Errorf("%w: %w", ErrInvalidState, err)
			}
		}
	case phaseAwaitingWaitID, phaseWaitingInput:
		if state.PendingResponse == nil || state.ToolCheckpoint == nil || state.FinalOutput != nil || state.ModelCalls == 0 {
			return fmt.Errorf("%w: waiting phase requires a pending response and Tool checkpoint", ErrInvalidState)
		}
		if err := state.PendingResponse.Validate(); err != nil {
			return fmt.Errorf("%w: pending response: %w", ErrInvalidState, err)
		}
		calls, _, err := responseToolCalls(state.PendingResponse)
		if err != nil || len(calls) == 0 {
			return fmt.Errorf("%w: pending response has no unambiguous tool calls", ErrInvalidState)
		}
		if err := state.ToolCheckpoint.validate(calls); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidState, err)
		}
		if state.Phase == phaseAwaitingWaitID && state.WaitID != nil {
			return fmt.Errorf("%w: awaiting_wait_id already has a WaitID", ErrInvalidState)
		}
		if state.Phase == phaseWaitingInput && (state.WaitID == nil || !state.WaitID.Valid()) {
			return fmt.Errorf("%w: waiting_input requires an Engine WaitID", ErrInvalidState)
		}
	case phaseCompleted:
		if state.PendingResponse != nil || state.ToolCheckpoint != nil || state.WaitID != nil || state.FinalOutput == nil || state.ModelCalls == 0 {
			return fmt.Errorf("%w: completed state requires only its final Output", ErrInvalidState)
		}
		if err := state.FinalOutput.Validate(); err != nil {
			return fmt.Errorf("%w: final Output: %w", ErrInvalidState, err)
		}
		if state.FinalOutput.ModelCalls != state.ModelCalls {
			return fmt.Errorf("%w: final Output model-call count does not match state", ErrInvalidState)
		}
	}
	return nil
}

func cloneMessages(messages []chat.Message) []chat.Message {
	cloned := make([]chat.Message, len(messages))
	for index := range messages {
		cloned[index] = messages[index].Clone()
	}
	return cloned
}

func cloneResponse(response *chat.Response) *chat.Response {
	return response.Clone()
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains multiple values")
		}
		return fmt.Errorf("decode trailing JSON value: %w", err)
	}
	return nil
}

func responseToolCalls(response *chat.Response) ([]chat.ToolCall, *chat.Message, error) {
	if response == nil {
		return nil, nil, errors.New("interaction: model returned a nil response")
	}
	if err := response.Validate(); err != nil {
		return nil, nil, fmt.Errorf("interaction: invalid model response: %w", err)
	}
	var calls []chat.ToolCall
	var message *chat.Message
	seenCallIDs := make(map[string]struct{})
	for index := range response.Choices {
		choice := &response.Choices[index]
		if choice.Message == nil {
			continue
		}
		var choiceCalls []chat.ToolCall
		for _, part := range choice.Message.Parts {
			if part.Kind == chat.PartToolCall {
				if _, duplicate := seenCallIDs[part.ToolCall.ID]; duplicate {
					return nil, nil, fmt.Errorf("interaction: duplicate tool call ID %q", part.ToolCall.ID)
				}
				seenCallIDs[part.ToolCall.ID] = struct{}{}
				choiceCalls = append(choiceCalls, *part.ToolCall)
			}
		}
		if len(choiceCalls) == 0 {
			continue
		}
		if len(calls) > 0 {
			return nil, nil, errors.New("interaction: multiple response choices request tools")
		}
		calls = choiceCalls
		cloned := choice.Message.Clone()
		message = &cloned
	}
	return calls, message, nil
}
