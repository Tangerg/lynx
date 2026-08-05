package interaction

import (
	"context"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/core/chat"
)

type execution struct {
	maxModelCalls uint32
	state         executionState
}

// Step advances exactly one pure Interaction boundary. Model and tool I/O are
// represented as dispatcher Effects and therefore never occur in this method.
func (execution *execution) Step(_ context.Context, signals []agent.Signal) (agent.Transition, error) {
	if execution == nil || execution.maxModelCalls == 0 {
		return agent.Transition{}, ErrInvalidState
	}
	if err := execution.state.Validate(execution.maxModelCalls); err != nil {
		return agent.Transition{}, err
	}
	switch execution.state.Phase {
	case phaseReadyModel:
		if len(signals) != 0 {
			return agent.Transition{}, fmt.Errorf("%w: unexpected Signal before first model call", ErrInvalidState)
		}
		return execution.requestModel(0)
	case phaseAwaitingModel:
		return execution.acceptModel(signals)
	case phaseAwaitingTools:
		return execution.acceptTools(signals)
	case phaseCompleted:
		return agent.Transition{}, fmt.Errorf("%w: completed execution cannot advance", ErrInvalidState)
	default:
		return agent.Transition{}, ErrInvalidState
	}
}

// Snapshot returns a complete, self-sufficient WorkingContext and checkpoint.
func (execution *execution) Snapshot() (agent.ExecutionState, error) {
	if execution == nil || execution.maxModelCalls == 0 {
		return agent.ExecutionState{}, ErrInvalidState
	}
	if err := execution.state.Validate(execution.maxModelCalls); err != nil {
		return agent.ExecutionState{}, err
	}
	return encodeState(execution.state)
}

func (execution *execution) requestModel(consumed uint32) (agent.Transition, error) {
	if execution.state.ModelCalls >= execution.maxModelCalls {
		return execution.fail(
			consumed,
			agent.FailureKindExecution,
			"interaction.limit.model_calls",
			"Interaction reached its configured model-call limit before a final response",
		)
	}
	envelope, err := newModelEffect(execution.state.Request)
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
	execution.state.ModelCalls++
	execution.state.Phase = phaseAwaitingModel
	return agent.Continue(consumed, effect)
}

func (execution *execution) acceptModel(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, fmt.Errorf("%w: model settlement Signal is missing", ErrInvalidState)
	}
	envelope, err := decodeSignal(signals[0].Payload())
	if err != nil {
		return agent.Transition{}, err
	}
	if envelope.Operation != operationModelCall {
		return agent.Transition{}, fmt.Errorf("%w: got %q while awaiting model result", ErrInvalidState, envelope.Operation)
	}
	if envelope.ModelResult.Error != "" {
		return execution.fail(
			1,
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
		output := Output{Response: *response, ModelCalls: execution.state.ModelCalls}
		if err := output.Validate(); err != nil {
			return agent.Transition{}, err
		}
		encoded, err := agent.EncodeOutput(output)
		if err != nil {
			return agent.Transition{}, err
		}
		execution.state.Phase = phaseCompleted
		execution.state.PendingResponse = response
		return agent.Complete(1, encoded)
	}

	effectEnvelope, err := newToolBatchEffect(calls)
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
	execution.state.Phase = phaseAwaitingTools
	execution.state.PendingResponse = response
	return agent.Continue(1, effect)
}

func (execution *execution) acceptTools(signals []agent.Signal) (agent.Transition, error) {
	if len(signals) == 0 {
		return agent.Transition{}, fmt.Errorf("%w: tool settlement Signal is missing", ErrInvalidState)
	}
	envelope, err := decodeSignal(signals[0].Payload())
	if err != nil {
		return agent.Transition{}, err
	}
	if envelope.Operation != operationToolBatch {
		return agent.Transition{}, fmt.Errorf("%w: got %q while awaiting tool results", ErrInvalidState, envelope.Operation)
	}
	calls, assistant, err := responseToolCalls(execution.state.PendingResponse)
	if err != nil {
		return agent.Transition{}, err
	}
	results := envelope.ToolResult.Results
	if err := validateToolResults(calls, results); err != nil {
		return agent.Transition{}, err
	}
	request := execution.state.Request.Clone()
	request.Messages = append(request.Messages, assistant.Clone(), chat.NewToolMessage(results...))
	if err := request.Validate(); err != nil {
		return agent.Transition{}, fmt.Errorf("%w: continuation request: %w", ErrInvalidState, err)
	}
	execution.state.Request = request
	execution.state.PendingResponse = nil
	execution.state.Phase = phaseReadyModel
	return execution.requestModel(1)
}

func validateToolResults(calls []chat.ToolCall, results []chat.ToolResult) error {
	if len(results) != len(calls) {
		return fmt.Errorf("%w: %d tool results do not match %d calls", ErrInvalidState, len(results), len(calls))
	}
	for index := range calls {
		if results[index].ID != calls[index].ID || results[index].Name != calls[index].Name {
			return fmt.Errorf("%w: tool result %d does not match call %q", ErrInvalidState, index, calls[index].ID)
		}
	}
	return nil
}

func (execution *execution) fail(
	consumed uint32,
	kind agent.FailureKind,
	code string,
	message string,
) (agent.Transition, error) {
	message = boundedDiagnostic(message)
	failure, err := agent.NewFailure(kind, code, message)
	if err != nil {
		return agent.Transition{}, err
	}
	return agent.Fail(consumed, failure)
}

func boundedDiagnostic(message string) string {
	if message == "" {
		return "Interaction operation failed"
	}
	const limit = 2048
	if len(message) <= limit {
		return message
	}
	return message[:limit]
}

var _ agent.Execution = (*execution)(nil)
