package interaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

type execution struct {
	definition *Definition
	state      executionState
}

// Step advances exactly one pure Interaction boundary. Model and tool I/O are
// represented as dispatcher Effects and therefore never occur in this method.
func (execution *execution) Step(_ context.Context, signals []agent.Signal) (agent.Transition, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.Transition{}, ErrInvalidExecutionState
	}
	if err := execution.state.Validate(execution.definition); err != nil {
		return agent.Transition{}, err
	}
	switch execution.state.Phase {
	case phaseReadyModel:
		steering, consumedSignals, err := collectSteeringSignals(signals)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.addSteering(steering)
		if err := execution.applySteering(); err != nil {
			return agent.Transition{}, err
		}
		return execution.requestModel(consumedSignals)
	case phaseAwaitingModel:
		return execution.acceptModel(signals)
	case phaseAwaitingTools:
		return execution.acceptTools(signals)
	case phaseAwaitingWaitID:
		return execution.acceptWaitID(signals)
	case phaseWaitingInput:
		return execution.acceptInputResponse(signals)
	case phaseAwaitingDelegateStarts:
		return execution.acceptDelegateStarts(signals)
	case phaseAwaitingDelegateWaitID:
		return execution.acceptDelegateWaitID(signals)
	case phaseWaitingDelegates:
		return execution.acceptDelegates(signals)
	case phaseCompleted:
		return agent.Transition{}, fmt.Errorf("%w: completed execution cannot advance", ErrInvalidExecutionState)
	default:
		return agent.Transition{}, ErrInvalidExecutionState
	}
}

// Snapshot returns a complete, self-sufficient WorkingContext and checkpoint.
func (execution *execution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || !execution.definition.valid() {
		return agent.ExecutionState{}, ErrInvalidExecutionState
	}
	if err := execution.state.Validate(execution.definition); err != nil {
		return agent.ExecutionState{}, err
	}
	return encodeState(execution.state)
}

func (execution *execution) requestModel(consumedSignals uint32) (agent.Transition, error) {
	if execution.state.ModelCallCount >= execution.definition.maxModelCalls {
		return execution.fail(
			consumedSignals,
			agent.FailureKindExecution,
			"interaction.limit.model_calls",
			"Interaction reached its configured model-call limit before a final response",
		)
	}
	modelCallSequence := execution.state.ModelCallCount + 1
	envelope, err := newModelEffect(
		execution.state.WorkingContext,
		modelCallSequence,
		execution.state.AdvertisedToolNames,
	)
	if err != nil {
		return agent.Transition{}, err
	}
	payload, err := encodeProtocol(envelope)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.NewDispatcherEffect(payload)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.ModelCallCount = modelCallSequence
	execution.state.Phase = phaseAwaitingModel
	return agent.Continue(consumedSignals, effect)
}

func (execution *execution) acceptModel(signals []agent.Signal) (agent.Transition, error) {
	envelope, steering, consumedSignals, err := collectExpectedSignal(signals, operationModelCall)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.addSteering(steering)
	if envelope.ModelResult.Error != "" {
		execution.state.SteeringMessages = nil
		return execution.fail(
			consumedSignals,
			agent.FailureKindExternal,
			"interaction.model.failed",
			envelope.ModelResult.Error,
		)
	}
	response := cloneResponse(envelope.ModelResult.Response)
	calls, _, err := responseToolCalls(response)
	if err != nil {
		return agent.Transition{}, err
	}
	if len(calls) == 0 {
		if len(execution.state.SteeringMessages) > 0 {
			choice := response.First()
			if choice == nil || choice.Message == nil || choice.FinishReason == "" {
				return agent.Transition{}, fmt.Errorf("%w: steered response has no finished assistant message", ErrInvalidExecutionState)
			}
			request := execution.state.WorkingContext.Clone()
			request.Messages = append(request.Messages, choice.Message.Clone())
			execution.state.WorkingContext = request
			if err := execution.applySteering(); err != nil {
				return agent.Transition{}, err
			}
			execution.state.Phase = phaseReadyModel
			return execution.requestModel(consumedSignals)
		}
		choice := response.First()
		if choice == nil || choice.Message == nil || choice.FinishReason == "" {
			return agent.Transition{}, fmt.Errorf("%w: final response has no finished assistant message", ErrInvalidExecutionState)
		}
		return execution.finishOrRetry(consumedSignals, Output{
			Source:        CompletionSourceModelResponse,
			ModelResponse: response,
			ModelCalls:    execution.state.ModelCallCount,
		}, []chat.Message{choice.Message.Clone()})
	}

	execution.state.PendingModelResponse = response
	execution.state.NextToolCallIndex = 0
	execution.state.ActiveToolCallEndIndex = 0
	execution.state.SettledToolResults = nil
	execution.state.DirectToolResultEligible = true
	return execution.advanceToolCallBatch(consumedSignals)
}

func (execution *execution) acceptTools(signals []agent.Signal) (agent.Transition, error) {
	envelope, steering, consumedSignals, err := collectExpectedSignal(signals, operationToolBatch)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.addSteering(steering)
	calls, _, err := execution.activeCallSegment()
	if err != nil {
		return agent.Transition{}, err
	}
	results := envelope.ToolResult.Results
	if checkpoint := envelope.ToolResult.Checkpoint; checkpoint != nil {
		if err := checkpoint.validate(calls); err != nil {
			return agent.Transition{}, err
		}
		return execution.requestInputWait(consumedSignals, checkpoint)
	}
	if err := validateToolResults(calls, results); err != nil {
		return agent.Transition{}, err
	}
	advertisedToolNames, err := mergeAdvertisedToolNames(
		execution.state.AdvertisedToolNames,
		envelope.ToolResult.AdvertisedToolNames,
	)
	if err != nil {
		return agent.Transition{}, fmt.Errorf("%w: advertised Tools: %w", ErrInvalidExecutionState, err)
	}
	execution.state.AdvertisedToolNames = advertisedToolNames
	execution.state.SettledToolResults = append(execution.state.SettledToolResults, results...)
	execution.state.NextToolCallIndex = execution.state.ActiveToolCallEndIndex
	execution.state.DirectToolResultEligible = execution.state.DirectToolResultEligible && envelope.ToolResult.Direct
	execution.state.ToolCheckpoint = nil
	execution.state.WaitID = nil
	return execution.advanceToolCallBatch(consumedSignals)
}

func (execution *execution) requestInputWait(
	consumedSignals uint32,
	checkpoint *toolCheckpoint,
) (agent.Transition, error) {
	if checkpoint == nil {
		return agent.Transition{}, fmt.Errorf("%w: missing Tool checkpoint", ErrInvalidExecutionState)
	}
	calls, _, err := execution.activeCallSegment()
	if err != nil {
		return agent.Transition{}, err
	}
	if err := checkpoint.validate(calls); err != nil {
		return agent.Transition{}, err
	}
	waitOpened := checkpoint.InputRequest
	payload, err := encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationWaitOpened,
		WaitOpened:    &waitOpened,
	})
	if err != nil {
		return agent.Transition{}, err
	}
	waitKey, err := checkpointWaitKey(execution.state.ModelCallCount, calls[checkpoint.NextToolCallIndex].ID, checkpoint.PauseCount)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.RequestWait(waitKey, payload)
	if err != nil {
		return agent.Transition{}, err
	}
	cloned := checkpoint.clone()
	execution.state.ToolCheckpoint = &cloned
	execution.state.WaitID = nil
	execution.state.Phase = phaseAwaitingWaitID
	return agent.Continue(consumedSignals, effect)
}

func (execution *execution) acceptWaitID(signals []agent.Signal) (agent.Transition, error) {
	envelope, steering, consumedSignals, err := collectExpectedSignal(signals, operationWaitOpened)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.addSteering(steering)
	var waitID agent.WaitID
	var ok bool
	for _, signal := range signals {
		decoded, decodeErr := decodeSignal(signal.Payload())
		if decodeErr == nil && decoded.Operation == operationWaitOpened {
			waitID, ok = signal.WaitID()
			break
		}
	}
	if !ok {
		return agent.Transition{}, fmt.Errorf("%w: Engine did not attach a WaitID", ErrInvalidExecutionState)
	}
	want, err := execution.state.ToolCheckpoint.InputRequest.inputRequest()
	if err != nil {
		return agent.Transition{}, err
	}
	got, err := envelope.WaitOpened.inputRequest()
	if err != nil || !sameInputRequest(want, got) {
		return agent.Transition{}, fmt.Errorf("%w: wait-opened payload does not match Tool checkpoint", ErrInvalidExecutionState)
	}
	execution.state.WaitID = &waitID
	execution.state.Phase = phaseWaitingInput
	return agent.Wait(consumedSignals, waitID)
}

func (execution *execution) acceptInputResponse(signals []agent.Signal) (agent.Transition, error) {
	envelope, steering, consumedSignals, err := collectExpectedSignal(signals, operationInputResponse)
	if err != nil {
		return agent.Transition{}, err
	}
	if len(steering) != 0 {
		return agent.Transition{}, fmt.Errorf("%w: steer cannot address an input wait", ErrInvalidExecutionState)
	}
	inputSignal, ok := signalForOperation(signals, operationInputResponse)
	if !ok {
		return agent.Transition{}, fmt.Errorf("%w: input-response Signal is missing", ErrInvalidExecutionState)
	}
	waitID, addressed := inputSignal.WaitID()
	if !addressed || execution.state.WaitID == nil || waitID != *execution.state.WaitID {
		return agent.Transition{}, fmt.Errorf("%w: input response addressed the wrong wait", ErrInvalidExecutionState)
	}
	request, err := execution.state.ToolCheckpoint.InputRequest.inputRequest()
	if err != nil {
		return agent.Transition{}, err
	}
	response, err := request.validateResponse(envelope.InputResponse)
	if err != nil {
		return agent.Transition{}, err
	}
	calls, _, err := execution.activeCallSegment()
	if err != nil {
		return agent.Transition{}, err
	}
	effectEnvelope, err := newToolBatchEffect(
		execution.state.ModelCallCount,
		execution.state.NextToolCallIndex,
		calls,
		execution.state.ToolCheckpoint,
		response,
	)
	if err != nil {
		return agent.Transition{}, err
	}
	payload, err := encodeProtocol(effectEnvelope)
	if err != nil {
		return agent.Transition{}, err
	}
	effect, err := agent.NewDispatcherEffect(payload)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.WaitID = nil
	execution.state.Phase = phaseAwaitingTools
	return agent.Continue(consumedSignals, effect)
}

func (execution *execution) complete(consumedSignals uint32, output Output) (agent.Transition, error) {
	if err := output.Validate(); err != nil {
		return agent.Transition{}, err
	}
	encoded, err := agent.EncodeOutput(output)
	if err != nil {
		return agent.Transition{}, err
	}
	execution.state.Phase = phaseCompleted
	execution.clearToolCallBatch()
	execution.state.WaitID = nil
	execution.state.SteeringMessages = nil
	execution.state.FinalOutput = &output
	return agent.Complete(consumedSignals, encoded)
}

func (execution *execution) finishOrRetry(
	consumedSignals uint32,
	output Output,
	completionContext []chat.Message,
) (agent.Transition, error) {
	if err := output.Validate(); err != nil {
		return agent.Transition{}, err
	}
	candidate := CompletionCandidate{
		workingContext: execution.state.WorkingContext.Clone(),
		output:         cloneOutput(output),
		artifacts:      newArtifacts(execution.state.ArtifactRecords),
	}
	decision := CompletionDecision{Accepted: true}
	if execution.definition.completionValidator != nil {
		var err error
		decision, err = execution.definition.completionValidator(candidate)
		if err != nil {
			return execution.fail(
				consumedSignals,
				agent.FailureKindExecution,
				"interaction.completion.validator_failed",
				err.Error(),
			)
		}
	}
	if !decision.Valid() {
		return execution.fail(
			consumedSignals,
			agent.FailureKindContract,
			"interaction.completion.decision_invalid",
			"CompletionValidator returned an invalid decision",
		)
	}
	if decision.Accepted {
		return execution.complete(consumedSignals, output)
	}
	request := execution.state.WorkingContext.Clone()
	request.Messages = append(request.Messages, cloneMessages(completionContext)...)
	request.Messages = append(request.Messages, chat.NewUserMessage(chat.NewTextPart(decision.Feedback)))
	if err := request.Validate(); err != nil {
		return agent.Transition{}, fmt.Errorf("%w: completion retry request: %w", ErrInvalidExecutionState, err)
	}
	execution.state.WorkingContext = request
	execution.clearToolCallBatch()
	execution.state.WaitID = nil
	execution.state.SteeringMessages = nil
	execution.state.Phase = phaseReadyModel
	return execution.requestModel(consumedSignals)
}

func (execution *execution) advanceToolCallBatch(consumedSignals uint32) (agent.Transition, error) {
	for {
		calls, assistant, err := responseToolCalls(execution.state.PendingModelResponse)
		if err != nil || uint64(len(calls)) > uint64(^uint32(0)) ||
			uint64(execution.state.NextToolCallIndex) > uint64(len(calls)) {
			return agent.Transition{}, fmt.Errorf("%w: invalid pending ToolCall batch", ErrInvalidExecutionState)
		}
		if execution.state.NextToolCallIndex == uint32(len(calls)) {
			results := append([]chat.ToolResult(nil), execution.state.SettledToolResults...)
			if execution.state.DirectToolResultEligible && len(execution.state.SteeringMessages) == 0 {
				return execution.finishOrRetry(consumedSignals, Output{
					Source:            CompletionSourceDirectToolResults,
					DirectToolResults: results, ModelCalls: execution.state.ModelCallCount,
				}, []chat.Message{assistant.Clone(), chat.NewToolMessage(results...)})
			}
			request := execution.state.WorkingContext.Clone()
			request.Messages = append(request.Messages, assistant.Clone(), chat.NewToolMessage(results...))
			execution.state.WorkingContext = request
			execution.clearToolCallBatch()
			if err := execution.applySteering(); err != nil {
				return agent.Transition{}, err
			}
			if err := execution.state.WorkingContext.Validate(); err != nil {
				return agent.Transition{}, fmt.Errorf("%w: continuation request: %w", ErrInvalidExecutionState, err)
			}
			execution.state.Phase = phaseReadyModel
			return execution.requestModel(consumedSignals)
		}
		if _, delegated := execution.definition.delegate(calls[execution.state.NextToolCallIndex].Name); delegated {
			execution.state.DirectToolResultEligible = false
			transition, started, err := execution.startDelegateSegment(consumedSignals, calls)
			if err != nil {
				return agent.Transition{}, err
			}
			if started {
				return transition, nil
			}
			continue
		}
		end := execution.state.NextToolCallIndex + 1
		for end < uint32(len(calls)) {
			if _, delegated := execution.definition.delegate(calls[end].Name); delegated {
				break
			}
			end++
		}
		effectEnvelope, err := newToolBatchEffect(
			execution.state.ModelCallCount,
			execution.state.NextToolCallIndex,
			calls[execution.state.NextToolCallIndex:end],
			nil,
			nil,
		)
		if err != nil {
			return agent.Transition{}, err
		}
		payload, err := encodeProtocol(effectEnvelope)
		if err != nil {
			return agent.Transition{}, err
		}
		effect, err := agent.NewDispatcherEffect(payload)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.ActiveToolCallEndIndex = end
		execution.state.Phase = phaseAwaitingTools
		return agent.Continue(consumedSignals, effect)
	}
}

func (execution *execution) activeCallSegment() ([]chat.ToolCall, *chat.Message, error) {
	calls, assistant, err := responseToolCalls(execution.state.PendingModelResponse)
	if err != nil || execution.state.NextToolCallIndex >= execution.state.ActiveToolCallEndIndex ||
		uint64(execution.state.ActiveToolCallEndIndex) > uint64(len(calls)) {
		return nil, nil, fmt.Errorf("%w: invalid active ToolCall segment", ErrInvalidExecutionState)
	}
	return calls[execution.state.NextToolCallIndex:execution.state.ActiveToolCallEndIndex], assistant, nil
}

func (execution *execution) clearToolCallBatch() {
	execution.state.PendingModelResponse = nil
	execution.state.NextToolCallIndex = 0
	execution.state.ActiveToolCallEndIndex = 0
	execution.state.SettledToolResults = nil
	execution.state.DirectToolResultEligible = false
	execution.state.ToolCheckpoint = nil
	execution.state.DelegateSegment = nil
}

func (execution *execution) addSteering(messages []chat.Message) {
	execution.state.SteeringMessages = append(execution.state.SteeringMessages, cloneMessages(messages)...)
}

func (execution *execution) applySteering() error {
	if len(execution.state.SteeringMessages) == 0 {
		return nil
	}
	request := execution.state.WorkingContext.Clone()
	request.Messages = append(request.Messages, cloneMessages(execution.state.SteeringMessages)...)
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: steered model request: %w", ErrInvalidExecutionState, err)
	}
	execution.state.WorkingContext = request
	execution.state.SteeringMessages = nil
	return nil
}

func collectSteeringSignals(signals []agent.Signal) ([]chat.Message, uint32, error) {
	var messages []chat.Message
	for _, signal := range signals {
		if _, addressed := signal.WaitID(); addressed {
			return nil, 0, fmt.Errorf("%w: steer Signal must not address a wait", ErrInvalidExecutionState)
		}
		envelope, err := decodeSignal(signal.Payload())
		if err != nil {
			return nil, 0, err
		}
		if envelope.Operation != operationSteer {
			return nil, 0, fmt.Errorf("%w: unexpected %q Signal", ErrInvalidExecutionState, envelope.Operation)
		}
		messages = append(messages, cloneMessages(envelope.Steer.Messages)...)
	}
	return messages, uint32(len(signals)), nil
}

func collectExpectedSignal(
	signals []agent.Signal,
	expected operation,
) (signalEnvelope, []chat.Message, uint32, error) {
	var result signalEnvelope
	var found bool
	var steering []chat.Message
	for _, signal := range signals {
		envelope, err := decodeSignal(signal.Payload())
		if err != nil {
			return signalEnvelope{}, nil, 0, err
		}
		switch envelope.Operation {
		case operationSteer:
			if _, addressed := signal.WaitID(); addressed {
				return signalEnvelope{}, nil, 0, fmt.Errorf("%w: steer Signal must not address a wait", ErrInvalidExecutionState)
			}
			steering = append(steering, cloneMessages(envelope.Steer.Messages)...)
		case expected:
			if found {
				return signalEnvelope{}, nil, 0, fmt.Errorf("%w: duplicate %q Signal", ErrInvalidExecutionState, expected)
			}
			_, addressed := signal.WaitID()
			requiresAddress := expected == operationWaitOpened || expected == operationInputResponse
			if addressed != requiresAddress {
				return signalEnvelope{}, nil, 0, fmt.Errorf("%w: %q Signal has invalid wait addressing", ErrInvalidExecutionState, expected)
			}
			found = true
			result = envelope
		default:
			return signalEnvelope{}, nil, 0, fmt.Errorf("%w: got %q while awaiting %q", ErrInvalidExecutionState, envelope.Operation, expected)
		}
	}
	if !found {
		return signalEnvelope{}, nil, 0, fmt.Errorf("%w: %q settlement Signal is missing", ErrInvalidExecutionState, expected)
	}
	return result, steering, uint32(len(signals)), nil
}

func signalForOperation(signals []agent.Signal, expected operation) (agent.Signal, bool) {
	for _, signal := range signals {
		envelope, err := decodeSignal(signal.Payload())
		if err == nil && envelope.Operation == expected {
			return signal, true
		}
	}
	return agent.Signal{}, false
}

func checkpointWaitKey(modelCallCount uint32, toolCallID string, pauseCount uint32) (agent.WaitKey, error) {
	hash := sha256.New()
	hash.Write([]byte(strconv.FormatUint(uint64(modelCallCount), 10)))
	hash.Write([]byte{0})
	hash.Write([]byte(toolCallID))
	hash.Write([]byte{0})
	hash.Write([]byte(strconv.FormatUint(uint64(pauseCount), 10)))
	return agent.ParseWaitKey("interaction.input." + hex.EncodeToString(hash.Sum(nil)))
}

func sameInputRequest(left, right ToolInputRequest) bool {
	return string(left.Prompt()) == string(right.Prompt()) &&
		string(left.ResponseSchema()) == string(right.ResponseSchema()) &&
		string(left.ContinuationState()) == string(right.ContinuationState())
}

func validateToolResults(calls []chat.ToolCall, results []chat.ToolResult) error {
	if len(results) != len(calls) {
		return fmt.Errorf("%w: %d tool results do not match %d calls", ErrInvalidExecutionState, len(results), len(calls))
	}
	for index := range calls {
		if results[index].ID != calls[index].ID || results[index].Name != calls[index].Name {
			return fmt.Errorf("%w: tool result %d does not match call %q", ErrInvalidExecutionState, index, calls[index].ID)
		}
	}
	return nil
}

func (execution *execution) fail(
	consumedSignals uint32,
	kind agent.FailureKind,
	code string,
	message string,
) (agent.Transition, error) {
	message = boundedDiagnostic(message)
	failure, err := agent.NewFailure(kind, code, message)
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Fail(consumedSignals, failure)
}

func boundedDiagnostic(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Interaction operation failed"
	}
	const limit = 2048
	if len(message) <= limit {
		return message
	}
	message = message[:limit]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "Interaction operation failed"
	}
	return message
}

var _ agent.Execution = (*execution)(nil)
