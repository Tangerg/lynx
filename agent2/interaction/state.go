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
	phaseReadyModel             phase = "ready_model"
	phaseAwaitingModel          phase = "awaiting_model"
	phaseAwaitingTools          phase = "awaiting_tools"
	phaseAwaitingWaitID         phase = "awaiting_wait_id"
	phaseWaitingInput           phase = "waiting_input"
	phaseAwaitingDelegateStarts phase = "awaiting_delegate_starts"
	phaseAwaitingDelegateWaitID phase = "awaiting_delegate_wait_id"
	phaseWaitingDelegates       phase = "waiting_delegates"
	phaseCompleted              phase = "completed"
)

func (value phase) valid() bool {
	switch value {
	case phaseReadyModel, phaseAwaitingModel, phaseAwaitingTools,
		phaseAwaitingWaitID, phaseWaitingInput, phaseAwaitingDelegateStarts,
		phaseAwaitingDelegateWaitID, phaseWaitingDelegates, phaseCompleted:
		return true
	default:
		return false
	}
}

// executionState is the complete Strategy-owned recovery state. Request is the
// self-sufficient WorkingContext for the next model call. PendingResponse is
// present only while a model-requested ToolCall batch is being settled.
type executionState struct {
	Phase                phase                 `json:"phase"`
	Request              *chat.Request         `json:"request"`
	ModelCalls           uint32                `json:"model_calls"`
	PendingResponse      *chat.Response        `json:"pending_response,omitempty"`
	NextCall             uint32                `json:"next_call,omitempty"`
	ActiveCallEnd        uint32                `json:"active_call_end,omitempty"`
	SettledResults       []chat.ToolResult     `json:"settled_results,omitempty"`
	DirectResultEligible bool                  `json:"direct_result_eligible,omitempty"`
	ToolCheckpoint       *toolCheckpoint       `json:"tool_checkpoint,omitempty"`
	DelegateSegment      *delegateSegmentState `json:"delegate_segment,omitempty"`
	WaitID               *agent.WaitID         `json:"wait_id,omitempty"`
	Steering             []chat.Message        `json:"steering,omitempty"`
	Artifacts            []artifactRecord      `json:"artifacts,omitempty"`
	FinalOutput          *Output               `json:"final_output,omitempty"`
}

type artifactRecord struct {
	ModelCall    uint32       `json:"model_call"`
	CallIndex    uint32       `json:"call_index"`
	CallID       string       `json:"call_id"`
	DelegateName string       `json:"delegate_name"`
	Output       agent.Output `json:"output"`
}

type delegateInvocationState struct {
	Key       *agent.ChildKey  `json:"key,omitempty"`
	ProcessID *agent.ProcessID `json:"process_id,omitempty"`
	Result    *chat.ToolResult `json:"result,omitempty"`
}

type delegateSegmentState struct {
	Invocations []delegateInvocationState `json:"invocations"`
}

func (state executionState) Validate(definition *Definition) error {
	if !definition.valid() {
		return ErrInvalidState
	}
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
	if state.ModelCalls > definition.maxModelCalls {
		return fmt.Errorf("%w: model call count exceeds configured limit", ErrInvalidState)
	}
	if err := state.validateArtifacts(definition); err != nil {
		return err
	}
	if len(state.Steering) > 0 {
		if err := validateSteeringMessages(state.Steering); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidState, err)
		}
	}
	switch state.Phase {
	case phaseReadyModel:
		if state.hasPendingBatch() || state.WaitID != nil || len(state.Steering) != 0 || state.FinalOutput != nil {
			return fmt.Errorf("%w: ready_model has inconsistent pending response or limit", ErrInvalidState)
		}
	case phaseAwaitingModel:
		if state.hasPendingBatch() || state.WaitID != nil || len(state.Steering) != 0 || state.FinalOutput != nil || state.ModelCalls == 0 {
			return fmt.Errorf("%w: awaiting_model has inconsistent pending response or limit", ErrInvalidState)
		}
	case phaseAwaitingTools, phaseAwaitingWaitID, phaseWaitingInput,
		phaseAwaitingDelegateStarts, phaseAwaitingDelegateWaitID, phaseWaitingDelegates:
		calls, err := state.validatePendingBatch(definition)
		if err != nil {
			return err
		}
		active := calls[state.NextCall:state.ActiveCallEnd]
		delegateSegment := state.Phase == phaseAwaitingDelegateStarts ||
			state.Phase == phaseAwaitingDelegateWaitID || state.Phase == phaseWaitingDelegates
		for _, call := range active {
			_, delegated := definition.delegate(call.Name)
			if delegated != delegateSegment {
				return fmt.Errorf("%w: active call segment mixes Tool and Delegate ownership", ErrInvalidState)
			}
		}
		if delegateSegment {
			if state.ToolCheckpoint != nil || state.DelegateSegment == nil {
				return fmt.Errorf("%w: Delegate phase has inconsistent batch state", ErrInvalidState)
			}
			if err := state.DelegateSegment.validate(state.Phase, active); err != nil {
				return err
			}
		} else if state.DelegateSegment != nil {
			return fmt.Errorf("%w: Tool phase contains Delegate state", ErrInvalidState)
		}
		switch state.Phase {
		case phaseAwaitingTools:
			if state.WaitID != nil {
				return fmt.Errorf("%w: awaiting_tools contains a WaitID", ErrInvalidState)
			}
			if state.ToolCheckpoint != nil {
				if err := state.ToolCheckpoint.validate(active); err != nil {
					return fmt.Errorf("%w: %w", ErrInvalidState, err)
				}
			}
		case phaseAwaitingWaitID, phaseWaitingInput:
			if state.ToolCheckpoint == nil {
				return fmt.Errorf("%w: input waiting phase requires a Tool checkpoint", ErrInvalidState)
			}
			if err := state.ToolCheckpoint.validate(active); err != nil {
				return fmt.Errorf("%w: %w", ErrInvalidState, err)
			}
			if state.Phase == phaseAwaitingWaitID && state.WaitID != nil {
				return fmt.Errorf("%w: awaiting_wait_id already has a WaitID", ErrInvalidState)
			}
			if state.Phase == phaseWaitingInput && (state.WaitID == nil || !state.WaitID.Valid()) {
				return fmt.Errorf("%w: waiting_input requires an Engine WaitID", ErrInvalidState)
			}
		case phaseAwaitingDelegateStarts:
			if state.WaitID != nil {
				return fmt.Errorf("%w: awaiting Delegate starts contains a WaitID", ErrInvalidState)
			}
		case phaseAwaitingDelegateWaitID:
			if state.WaitID != nil {
				return fmt.Errorf("%w: awaiting Delegate wait contains a WaitID", ErrInvalidState)
			}
		case phaseWaitingDelegates:
			if state.WaitID == nil || !state.WaitID.Valid() {
				return fmt.Errorf("%w: waiting_delegates requires an Engine WaitID", ErrInvalidState)
			}
		}
	case phaseCompleted:
		if state.hasPendingBatch() || state.WaitID != nil || len(state.Steering) != 0 || state.FinalOutput == nil || state.ModelCalls == 0 {
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

func (state executionState) validateArtifacts(definition *Definition) error {
	var previousModelCall uint32
	var previousCallIndex uint32
	type artifactIdentity struct {
		modelCall uint32
		callID    string
	}
	seen := make(map[artifactIdentity]struct{}, len(state.Artifacts))
	for index, artifact := range state.Artifacts {
		delegate, found := definition.delegate(artifact.DelegateName)
		if artifact.ModelCall == 0 || artifact.ModelCall > state.ModelCalls ||
			artifact.CallID == "" || !found || !artifact.Output.Valid() {
			return fmt.Errorf("%w: artifact %d has invalid identity or output", ErrInvalidState, index)
		}
		if index > 0 && (artifact.ModelCall < previousModelCall ||
			artifact.ModelCall == previousModelCall && artifact.CallIndex <= previousCallIndex) {
			return fmt.Errorf("%w: artifacts are not in strict ToolCall order", ErrInvalidState)
		}
		identity := artifactIdentity{modelCall: artifact.ModelCall, callID: artifact.CallID}
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("%w: duplicate artifact ToolCall identity", ErrInvalidState)
		}
		seen[identity] = struct{}{}
		if err := delegate.outputSchema.ValidateOutput(artifact.Output); err != nil {
			return fmt.Errorf("%w: artifact %d violates Delegate output contract", ErrInvalidState, index)
		}
		previousModelCall = artifact.ModelCall
		previousCallIndex = artifact.CallIndex
	}
	return state.validateCurrentBatchArtifacts(definition)
}

func (state executionState) validateCurrentBatchArtifacts(definition *Definition) error {
	if len(state.Artifacts) == 0 || state.Artifacts[len(state.Artifacts)-1].ModelCall != state.ModelCalls {
		return nil
	}
	calls, _, err := responseToolCalls(state.PendingResponse)
	if err != nil {
		return fmt.Errorf("%w: current-round artifact has no pending ToolCall batch", ErrInvalidState)
	}
	for _, artifact := range state.Artifacts {
		if artifact.ModelCall != state.ModelCalls {
			continue
		}
		if artifact.CallIndex >= state.NextCall || uint64(artifact.CallIndex) >= uint64(len(calls)) {
			return fmt.Errorf("%w: current-round artifact is not settled", ErrInvalidState)
		}
		if uint64(artifact.CallIndex) >= uint64(len(state.SettledResults)) {
			return fmt.Errorf("%w: current-round artifact has no settled result", ErrInvalidState)
		}
		call := calls[artifact.CallIndex]
		if call.ID != artifact.CallID || call.Name != artifact.DelegateName {
			return fmt.Errorf("%w: current-round artifact does not match ToolCall", ErrInvalidState)
		}
		if _, found := definition.delegate(call.Name); !found {
			return fmt.Errorf("%w: current-round artifact is not a Delegate output", ErrInvalidState)
		}
		result := state.SettledResults[artifact.CallIndex]
		if result.IsError || result.ID != call.ID || result.Name != call.Name ||
			result.Result != string(artifact.Output.JSON()) {
			return fmt.Errorf("%w: current-round artifact does not match settled result", ErrInvalidState)
		}
	}
	return nil
}

func (state executionState) hasPendingBatch() bool {
	return state.PendingResponse != nil || state.NextCall != 0 || state.ActiveCallEnd != 0 ||
		len(state.SettledResults) != 0 || state.DirectResultEligible || state.ToolCheckpoint != nil ||
		state.DelegateSegment != nil
}

func (state executionState) validatePendingBatch(definition *Definition) ([]chat.ToolCall, error) {
	if state.PendingResponse == nil || state.FinalOutput != nil || state.ModelCalls == 0 {
		return nil, fmt.Errorf("%w: active call phase requires a model response", ErrInvalidState)
	}
	if err := state.PendingResponse.Validate(); err != nil {
		return nil, fmt.Errorf("%w: pending response: %w", ErrInvalidState, err)
	}
	calls, _, err := responseToolCalls(state.PendingResponse)
	if err != nil || len(calls) == 0 || uint64(len(calls)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("%w: pending response has no bounded unambiguous tool calls", ErrInvalidState)
	}
	if state.NextCall != uint32(len(state.SettledResults)) ||
		state.NextCall >= state.ActiveCallEnd || uint64(state.ActiveCallEnd) > uint64(len(calls)) {
		return nil, fmt.Errorf("%w: pending call cursor is inconsistent", ErrInvalidState)
	}
	if err := validateToolResults(calls[:state.NextCall], state.SettledResults); err != nil {
		return nil, err
	}
	if !state.DirectResultEligible && state.NextCall == 0 && state.Phase == phaseAwaitingTools {
		return nil, fmt.Errorf("%w: fresh Tool batch lost its direct-result candidate", ErrInvalidState)
	}
	return calls, nil
}

func (segment delegateSegmentState) validate(current phase, calls []chat.ToolCall) error {
	if len(segment.Invocations) != len(calls) {
		return fmt.Errorf("%w: Delegate batch length does not match calls", ErrInvalidState)
	}
	pending := 0
	started := 0
	for index, invocation := range segment.Invocations {
		if invocation.Key != nil && !invocation.Key.Valid() ||
			invocation.ProcessID != nil && !invocation.ProcessID.Valid() {
			return fmt.Errorf("%w: Delegate call %d has invalid Framework identity", ErrInvalidState, index)
		}
		if invocation.Result != nil {
			if err := invocation.Result.Validate(); err != nil ||
				invocation.Result.ID != calls[index].ID || invocation.Result.Name != calls[index].Name ||
				!invocation.Result.IsError {
				return fmt.Errorf("%w: Delegate result %d does not match its call", ErrInvalidState, index)
			}
		}
		switch {
		case invocation.Key != nil && invocation.ProcessID == nil && invocation.Result == nil:
			pending++
		case invocation.Key != nil && invocation.ProcessID != nil && invocation.Result == nil:
			started++
		case invocation.ProcessID == nil && invocation.Result != nil:
		default:
			return fmt.Errorf("%w: Delegate call %d has contradictory settlement", ErrInvalidState, index)
		}
	}
	if current == phaseAwaitingDelegateStarts && (pending == 0 || started != 0) ||
		current != phaseAwaitingDelegateStarts && pending != 0 ||
		(current == phaseAwaitingDelegateWaitID || current == phaseWaitingDelegates) && started == 0 {
		return fmt.Errorf("%w: Delegate batch phase does not match settlements", ErrInvalidState)
	}
	return nil
}

func (state executionState) validatePendingToolInput() error {
	if state.Phase != phaseWaitingInput || state.Request == nil || state.ModelCalls == 0 ||
		state.PendingResponse == nil || state.FinalOutput != nil || state.DelegateSegment != nil ||
		state.ToolCheckpoint == nil || state.WaitID == nil || !state.WaitID.Valid() {
		return ErrInvalidState
	}
	if err := state.Request.Validate(); err != nil || len(state.Request.Tools) != 0 {
		return ErrInvalidState
	}
	calls, _, err := responseToolCalls(state.PendingResponse)
	if err != nil || state.NextCall != uint32(len(state.SettledResults)) ||
		state.NextCall >= state.ActiveCallEnd || uint64(state.ActiveCallEnd) > uint64(len(calls)) {
		return ErrInvalidState
	}
	if err := validateToolResults(calls[:state.NextCall], state.SettledResults); err != nil {
		return err
	}
	return state.ToolCheckpoint.validate(calls[state.NextCall:state.ActiveCallEnd])
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
