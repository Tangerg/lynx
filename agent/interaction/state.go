package interaction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	agent "github.com/Tangerg/lynx/agent"
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

// executionState is the complete Strategy-owned recovery state. WorkingContext
// is self-sufficient for the next model call. PendingModelResponse is present
// only while a model-requested ToolCall batch is being settled.
type executionState struct {
	Phase                    phase                 `json:"phase"`
	WorkingContext           *chat.Request         `json:"working_context"`
	ModelCallCount           uint32                `json:"model_call_count"`
	AdvertisedToolNames      []string              `json:"advertised_tool_names,omitempty"`
	PendingModelResponse     *chat.Response        `json:"pending_model_response,omitempty"`
	NextToolCallIndex        uint32                `json:"next_tool_call_index,omitempty"`
	ActiveToolCallEndIndex   uint32                `json:"active_tool_call_end_index,omitempty"`
	SettledToolResults       []chat.ToolResult     `json:"settled_tool_results,omitempty"`
	DirectToolResultEligible bool                  `json:"direct_tool_result_eligible,omitempty"`
	ToolCheckpoint           *toolCheckpoint       `json:"tool_checkpoint,omitempty"`
	DelegateSegment          *delegateSegmentState `json:"delegate_segment,omitempty"`
	WaitID                   *agent.WaitID         `json:"wait_id,omitempty"`
	SteeringMessages         []chat.Message        `json:"steering_messages,omitempty"`
	ArtifactRecords          []artifactRecord      `json:"artifact_records,omitempty"`
	FinalOutput              *Output               `json:"final_output,omitempty"`
}

type artifactRecord struct {
	ModelCallSequence uint32       `json:"model_call_sequence"`
	ToolCallIndex     uint32       `json:"tool_call_index"`
	ToolCallID        string       `json:"tool_call_id"`
	DelegateName      string       `json:"delegate_name"`
	Output            agent.Output `json:"output"`
}

type delegateInvocationState struct {
	ChildKey       *agent.ChildKey  `json:"child_key,omitempty"`
	ChildProcessID *agent.ProcessID `json:"child_process_id,omitempty"`
	ToolResult     *chat.ToolResult `json:"tool_result,omitempty"`
}

type delegateSegmentState struct {
	Invocations []delegateInvocationState `json:"invocations"`
}

func (state executionState) Validate(definition *Definition) error {
	if err := state.validateEnvelope(definition); err != nil {
		return err
	}
	if err := state.validateArtifacts(definition); err != nil {
		return err
	}
	return state.validatePhaseState(definition)
}

func (state executionState) validateEnvelope(definition *Definition) error {
	if !definition.valid() {
		return ErrInvalidExecutionState
	}
	if !state.Phase.valid() {
		return fmt.Errorf("%w: unknown phase %q", ErrInvalidExecutionState, state.Phase)
	}
	if state.WorkingContext == nil {
		return fmt.Errorf("%w: WorkingContext is required", ErrInvalidExecutionState)
	}
	if err := state.WorkingContext.Validate(); err != nil {
		return fmt.Errorf("%w: WorkingContext: %w", ErrInvalidExecutionState, err)
	}
	if len(state.WorkingContext.Tools) != 0 {
		return fmt.Errorf("%w: executable tool definitions do not belong in WorkingContext", ErrInvalidExecutionState)
	}
	if state.ModelCallCount > definition.maxModelCalls {
		return fmt.Errorf("%w: model call count exceeds configured limit", ErrInvalidExecutionState)
	}
	if err := validateAdvertisedToolNames(state.AdvertisedToolNames); err != nil {
		return fmt.Errorf("%w: advertised Tools: %w", ErrInvalidExecutionState, err)
	}
	if len(state.SteeringMessages) > 0 {
		if err := validateSteeringMessages(state.SteeringMessages); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidExecutionState, err)
		}
	}
	return nil
}

func (state executionState) validatePhaseState(definition *Definition) error {
	switch state.Phase {
	case phaseReadyModel:
		return state.validateReadyModelState()
	case phaseAwaitingModel:
		return state.validateAwaitingModelState()
	case phaseAwaitingTools, phaseAwaitingWaitID, phaseWaitingInput,
		phaseAwaitingDelegateStarts, phaseAwaitingDelegateWaitID, phaseWaitingDelegates:
		return state.validateActiveCallState(definition)
	case phaseCompleted:
		return state.validateCompletedState()
	}
	return nil
}

func (state executionState) validateReadyModelState() error {
	if state.hasPendingBatch() || state.WaitID != nil || len(state.SteeringMessages) != 0 || state.FinalOutput != nil {
		return fmt.Errorf("%w: ready_model has inconsistent pending response or limit", ErrInvalidExecutionState)
	}
	return nil
}

func (state executionState) validateAwaitingModelState() error {
	if state.hasPendingBatch() || state.WaitID != nil || len(state.SteeringMessages) != 0 || state.FinalOutput != nil || state.ModelCallCount == 0 {
		return fmt.Errorf("%w: awaiting_model has inconsistent pending response or limit", ErrInvalidExecutionState)
	}
	return nil
}

func (state executionState) validateActiveCallState(definition *Definition) error {
	calls, err := state.validatePendingBatch()
	if err != nil {
		return err
	}
	active := calls[state.NextToolCallIndex:state.ActiveToolCallEndIndex]
	delegateSegment := state.delegatePhase()
	for _, call := range active {
		_, delegated := definition.delegate(call.Name)
		if delegated != delegateSegment {
			return fmt.Errorf("%w: active call segment mixes Tool and Delegate ownership", ErrInvalidExecutionState)
		}
	}
	if delegateSegment {
		return state.validateDelegateCallState(active)
	}
	return state.validateToolCallState(active)
}

func (state executionState) delegatePhase() bool {
	return state.Phase == phaseAwaitingDelegateStarts ||
		state.Phase == phaseAwaitingDelegateWaitID || state.Phase == phaseWaitingDelegates
}

func (state executionState) validateDelegateCallState(active []chat.ToolCall) error {
	if state.ToolCheckpoint != nil || state.DelegateSegment == nil {
		return fmt.Errorf("%w: Delegate phase has inconsistent batch state", ErrInvalidExecutionState)
	}
	if err := state.DelegateSegment.validate(state.Phase, active); err != nil {
		return err
	}
	switch state.Phase {
	case phaseAwaitingDelegateStarts:
		if state.WaitID != nil {
			return fmt.Errorf("%w: awaiting Delegate starts contains a WaitID", ErrInvalidExecutionState)
		}
	case phaseAwaitingDelegateWaitID:
		if state.WaitID != nil {
			return fmt.Errorf("%w: awaiting Delegate wait contains a WaitID", ErrInvalidExecutionState)
		}
	case phaseWaitingDelegates:
		if state.WaitID == nil || !state.WaitID.Valid() {
			return fmt.Errorf("%w: waiting_delegates requires an Engine WaitID", ErrInvalidExecutionState)
		}
	}
	return nil
}

func (state executionState) validateToolCallState(active []chat.ToolCall) error {
	if state.DelegateSegment != nil {
		return fmt.Errorf("%w: Tool phase contains Delegate state", ErrInvalidExecutionState)
	}
	switch state.Phase {
	case phaseAwaitingTools:
		if state.WaitID != nil {
			return fmt.Errorf("%w: awaiting_tools contains a WaitID", ErrInvalidExecutionState)
		}
		if state.ToolCheckpoint != nil {
			if err := state.ToolCheckpoint.validate(active); err != nil {
				return fmt.Errorf("%w: %w", ErrInvalidExecutionState, err)
			}
		}
	case phaseAwaitingWaitID, phaseWaitingInput:
		return state.validateInputWaitingState(active)
	}
	return nil
}

func (state executionState) validateInputWaitingState(active []chat.ToolCall) error {
	if state.ToolCheckpoint == nil {
		return fmt.Errorf("%w: input waiting phase requires a Tool checkpoint", ErrInvalidExecutionState)
	}
	if err := state.ToolCheckpoint.validate(active); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutionState, err)
	}
	if state.Phase == phaseAwaitingWaitID && state.WaitID != nil {
		return fmt.Errorf("%w: awaiting_wait_id already has a WaitID", ErrInvalidExecutionState)
	}
	if state.Phase == phaseWaitingInput && (state.WaitID == nil || !state.WaitID.Valid()) {
		return fmt.Errorf("%w: waiting_input requires an Engine WaitID", ErrInvalidExecutionState)
	}
	return nil
}

func (state executionState) validateCompletedState() error {
	if state.hasPendingBatch() || state.WaitID != nil || len(state.SteeringMessages) != 0 || state.FinalOutput == nil || state.ModelCallCount == 0 {
		return fmt.Errorf("%w: completed state requires only its final Output", ErrInvalidExecutionState)
	}
	if err := state.FinalOutput.Validate(); err != nil {
		return fmt.Errorf("%w: final Output: %w", ErrInvalidExecutionState, err)
	}
	if state.FinalOutput.ModelCalls != state.ModelCallCount {
		return fmt.Errorf("%w: final Output model-call count does not match state", ErrInvalidExecutionState)
	}
	return nil
}

func (state executionState) validateArtifacts(definition *Definition) error {
	var previousModelCallSequence uint32
	var previousToolCallIndex uint32
	type artifactIdentity struct {
		modelCallSequence uint32
		toolCallID        string
	}
	seen := make(map[artifactIdentity]struct{}, len(state.ArtifactRecords))
	for index, artifact := range state.ArtifactRecords {
		delegate, found := definition.delegate(artifact.DelegateName)
		if artifact.ModelCallSequence == 0 || artifact.ModelCallSequence > state.ModelCallCount ||
			artifact.ToolCallID == "" || !found || !artifact.Output.Valid() {
			return fmt.Errorf("%w: artifact %d has invalid identity or output", ErrInvalidExecutionState, index)
		}
		if index > 0 && (artifact.ModelCallSequence < previousModelCallSequence ||
			artifact.ModelCallSequence == previousModelCallSequence && artifact.ToolCallIndex <= previousToolCallIndex) {
			return fmt.Errorf("%w: artifacts are not in strict ToolCall order", ErrInvalidExecutionState)
		}
		identity := artifactIdentity{
			modelCallSequence: artifact.ModelCallSequence,
			toolCallID:        artifact.ToolCallID,
		}
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("%w: duplicate artifact ToolCall identity", ErrInvalidExecutionState)
		}
		seen[identity] = struct{}{}
		if err := delegate.outputSchema.ValidateOutput(artifact.Output); err != nil {
			return fmt.Errorf("%w: artifact %d violates Delegate output contract", ErrInvalidExecutionState, index)
		}
		previousModelCallSequence = artifact.ModelCallSequence
		previousToolCallIndex = artifact.ToolCallIndex
	}
	return state.validateCurrentBatchArtifacts(definition)
}

func (state executionState) validateCurrentBatchArtifacts(definition *Definition) error {
	if len(state.ArtifactRecords) == 0 || state.ArtifactRecords[len(state.ArtifactRecords)-1].ModelCallSequence != state.ModelCallCount {
		return nil
	}
	calls, _, err := responseToolCalls(state.PendingModelResponse)
	if err != nil {
		return fmt.Errorf("%w: current-round artifact has no pending ToolCall batch", ErrInvalidExecutionState)
	}
	for _, artifact := range state.ArtifactRecords {
		if artifact.ModelCallSequence != state.ModelCallCount {
			continue
		}
		if artifact.ToolCallIndex >= state.NextToolCallIndex || uint64(artifact.ToolCallIndex) >= uint64(len(calls)) {
			return fmt.Errorf("%w: current-round artifact is not settled", ErrInvalidExecutionState)
		}
		if uint64(artifact.ToolCallIndex) >= uint64(len(state.SettledToolResults)) {
			return fmt.Errorf("%w: current-round artifact has no settled result", ErrInvalidExecutionState)
		}
		call := calls[artifact.ToolCallIndex]
		if call.ID != artifact.ToolCallID || call.Name != artifact.DelegateName {
			return fmt.Errorf("%w: current-round artifact does not match ToolCall", ErrInvalidExecutionState)
		}
		if _, found := definition.delegate(call.Name); !found {
			return fmt.Errorf("%w: current-round artifact is not a Delegate output", ErrInvalidExecutionState)
		}
		result := state.SettledToolResults[artifact.ToolCallIndex]
		if result.IsError || result.ID != call.ID || result.Name != call.Name ||
			result.Result != string(artifact.Output.JSON()) {
			return fmt.Errorf("%w: current-round artifact does not match settled result", ErrInvalidExecutionState)
		}
	}
	return nil
}

func (state executionState) hasPendingBatch() bool {
	return state.PendingModelResponse != nil || state.NextToolCallIndex != 0 || state.ActiveToolCallEndIndex != 0 ||
		len(state.SettledToolResults) != 0 || state.DirectToolResultEligible || state.ToolCheckpoint != nil ||
		state.DelegateSegment != nil
}

func (state executionState) validatePendingBatch() ([]chat.ToolCall, error) {
	if state.PendingModelResponse == nil || state.FinalOutput != nil || state.ModelCallCount == 0 {
		return nil, fmt.Errorf("%w: active call phase requires a model response", ErrInvalidExecutionState)
	}
	if err := state.PendingModelResponse.Validate(); err != nil {
		return nil, fmt.Errorf("%w: pending response: %w", ErrInvalidExecutionState, err)
	}
	calls, _, err := responseToolCalls(state.PendingModelResponse)
	if err != nil || len(calls) == 0 || uint64(len(calls)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("%w: pending response has no bounded unambiguous tool calls", ErrInvalidExecutionState)
	}
	if state.NextToolCallIndex != uint32(len(state.SettledToolResults)) ||
		state.NextToolCallIndex >= state.ActiveToolCallEndIndex || uint64(state.ActiveToolCallEndIndex) > uint64(len(calls)) {
		return nil, fmt.Errorf("%w: ToolCall cursor is inconsistent", ErrInvalidExecutionState)
	}
	if err := validateToolResults(calls[:state.NextToolCallIndex], state.SettledToolResults); err != nil {
		return nil, err
	}
	if !state.DirectToolResultEligible && state.NextToolCallIndex == 0 && state.Phase == phaseAwaitingTools {
		return nil, fmt.Errorf("%w: fresh Tool batch lost its direct-result candidate", ErrInvalidExecutionState)
	}
	return calls, nil
}

func (segment delegateSegmentState) validate(current phase, calls []chat.ToolCall) error {
	if len(segment.Invocations) != len(calls) {
		return fmt.Errorf("%w: Delegate batch length does not match calls", ErrInvalidExecutionState)
	}
	pending := 0
	started := 0
	for index, invocation := range segment.Invocations {
		if invocation.ChildKey != nil && !invocation.ChildKey.Valid() ||
			invocation.ChildProcessID != nil && !invocation.ChildProcessID.Valid() {
			return fmt.Errorf("%w: Delegate call %d has invalid Framework identity", ErrInvalidExecutionState, index)
		}
		if invocation.ToolResult != nil {
			if err := invocation.ToolResult.Validate(); err != nil ||
				invocation.ToolResult.ID != calls[index].ID || invocation.ToolResult.Name != calls[index].Name ||
				!invocation.ToolResult.IsError {
				return fmt.Errorf("%w: Delegate result %d does not match its call", ErrInvalidExecutionState, index)
			}
		}
		switch {
		case invocation.ChildKey != nil && invocation.ChildProcessID == nil && invocation.ToolResult == nil:
			pending++
		case invocation.ChildKey != nil && invocation.ChildProcessID != nil && invocation.ToolResult == nil:
			started++
		case invocation.ChildProcessID == nil && invocation.ToolResult != nil:
		default:
			return fmt.Errorf("%w: Delegate call %d has contradictory settlement", ErrInvalidExecutionState, index)
		}
	}
	if current == phaseAwaitingDelegateStarts && (pending == 0 || started != 0) ||
		current != phaseAwaitingDelegateStarts && pending != 0 ||
		(current == phaseAwaitingDelegateWaitID || current == phaseWaitingDelegates) && started == 0 {
		return fmt.Errorf("%w: Delegate batch phase does not match settlements", ErrInvalidExecutionState)
	}
	return nil
}

func (state executionState) validatePendingToolInput() error {
	if state.Phase != phaseWaitingInput || state.WorkingContext == nil || state.ModelCallCount == 0 ||
		state.PendingModelResponse == nil || state.FinalOutput != nil || state.DelegateSegment != nil ||
		state.ToolCheckpoint == nil || state.WaitID == nil || !state.WaitID.Valid() {
		return ErrInvalidExecutionState
	}
	if err := state.WorkingContext.Validate(); err != nil || len(state.WorkingContext.Tools) != 0 {
		return ErrInvalidExecutionState
	}
	calls, _, err := responseToolCalls(state.PendingModelResponse)
	if err != nil || state.NextToolCallIndex != uint32(len(state.SettledToolResults)) ||
		state.NextToolCallIndex >= state.ActiveToolCallEndIndex || uint64(state.ActiveToolCallEndIndex) > uint64(len(calls)) {
		return ErrInvalidExecutionState
	}
	if err := validateToolResults(calls[:state.NextToolCallIndex], state.SettledToolResults); err != nil {
		return err
	}
	return state.ToolCheckpoint.validate(calls[state.NextToolCallIndex:state.ActiveToolCallEndIndex])
}

func (state executionState) activeDelegateCalls() ([]chat.ToolCall, error) {
	if state.Phase != phaseAwaitingDelegateStarts &&
		state.Phase != phaseAwaitingDelegateWaitID &&
		state.Phase != phaseWaitingDelegates ||
		state.WorkingContext == nil || state.ModelCallCount == 0 ||
		state.FinalOutput != nil || state.ToolCheckpoint != nil || state.DelegateSegment == nil {
		return nil, ErrInvalidExecutionState
	}
	if err := state.WorkingContext.Validate(); err != nil || len(state.WorkingContext.Tools) != 0 {
		return nil, ErrInvalidExecutionState
	}
	if len(state.SteeringMessages) > 0 {
		if err := validateSteeringMessages(state.SteeringMessages); err != nil {
			return nil, err
		}
	}
	calls, err := state.validatePendingBatch()
	if err != nil {
		return nil, err
	}
	active := calls[state.NextToolCallIndex:state.ActiveToolCallEndIndex]
	if err := state.DelegateSegment.validate(state.Phase, active); err != nil {
		return nil, err
	}
	switch state.Phase {
	case phaseAwaitingDelegateStarts, phaseAwaitingDelegateWaitID:
		if state.WaitID != nil {
			return nil, ErrInvalidExecutionState
		}
	case phaseWaitingDelegates:
		if state.WaitID == nil || !state.WaitID.Valid() {
			return nil, ErrInvalidExecutionState
		}
	}
	return active, nil
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
