package interaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
)

const protocolSchemaVersion uint16 = 5

type operation string

const (
	operationModelCall     operation = "model_call"
	operationToolBatch     operation = "tool_batch"
	operationWaitOpened    operation = "wait_opened"
	operationInputResponse operation = "input_response"
	operationSteer         operation = "steer"
)

func (value operation) valid() bool {
	return value == operationModelCall || value == operationToolBatch ||
		value == operationWaitOpened || value == operationInputResponse || value == operationSteer
}

type effectEnvelope struct {
	SchemaVersion uint16         `json:"schema_version"`
	Operation     operation      `json:"operation"`
	ModelCall     *modelCall     `json:"model_call,omitempty"`
	ToolBatch     *toolBatchCall `json:"tool_batch,omitempty"`
}

type modelCall struct {
	ModelCallSequence     uint32           `json:"model_call_sequence"`
	Request               chat.Request     `json:"request"`
	AdvertisedToolNames   []string         `json:"advertised_tool_names,omitempty"`
	AppliedSteerSignalIDs []agent.SignalID `json:"applied_steer_signal_ids,omitempty"`
}

type toolBatchCall struct {
	ModelCallSequence  uint32          `json:"model_call_sequence"`
	FirstToolCallIndex uint32          `json:"first_tool_call_index"`
	Calls              []chat.ToolCall `json:"calls"`
	Checkpoint         *toolCheckpoint `json:"checkpoint,omitempty"`
	InputResponse      json.RawMessage `json:"input_response,omitempty"`
}

type signalEnvelope struct {
	SchemaVersion uint16            `json:"schema_version"`
	Operation     operation         `json:"operation"`
	ModelResult   *modelCallResult  `json:"model_result,omitempty"`
	ToolResult    *toolBatchResult  `json:"tool_result,omitempty"`
	WaitOpened    *inputRequestWire `json:"wait_opened,omitempty"`
	InputResponse json.RawMessage   `json:"input_response,omitempty"`
	Steer         *steerInput       `json:"steer,omitempty"`
}

type modelCallResult struct {
	Response  *chat.Response `json:"response,omitempty"`
	Error     string         `json:"error,omitempty"`
	HostError string         `json:"host_error,omitempty"`
}

type steerInput struct {
	Messages []chat.Message `json:"messages"`
}

type toolBatchResult struct {
	Results             []chat.ToolResult `json:"results"`
	Direct              bool              `json:"direct"`
	AdvertisedToolNames []string          `json:"advertised_tool_names,omitempty"`
	Checkpoint          *toolCheckpoint   `json:"checkpoint,omitempty"`
	HostError           string            `json:"host_error,omitempty"`
}

type toolCheckpoint struct {
	CompletedResults    []chat.ToolResult `json:"completed_results"`
	AdvertisedToolNames []string          `json:"advertised_tool_names,omitempty"`
	NextToolCallIndex   uint32            `json:"next_tool_call_index"`
	PauseCount          uint32            `json:"pause_count"`
	InputRequest        inputRequestWire  `json:"input_request"`
}

type inputRequestWire struct {
	Prompt            json.RawMessage `json:"prompt"`
	ResponseSchema    json.RawMessage `json:"response_schema"`
	ContinuationState json.RawMessage `json:"continuation_state"`
}

func wireInputRequest(request ToolInputRequest) inputRequestWire {
	return inputRequestWire{
		Prompt: request.Prompt(), ResponseSchema: request.ResponseSchema(),
		ContinuationState: request.ContinuationState(),
	}
}

func (wire inputRequestWire) inputRequest() (ToolInputRequest, error) {
	return NewToolInputRequest(wire.Prompt, wire.ResponseSchema, wire.ContinuationState)
}

func newModelEffect(
	request *chat.Request,
	modelCallSequence uint32,
	advertisedToolNames []string,
	appliedSteerSignalIDs []agent.SignalID,
) (effectEnvelope, error) {
	if request == nil {
		return effectEnvelope{}, errors.New("interaction: model request is nil")
	}
	if modelCallSequence == 0 {
		return effectEnvelope{}, errors.New("interaction: model call sequence is required")
	}
	if err := validateAdvertisedToolNames(advertisedToolNames); err != nil {
		return effectEnvelope{}, fmt.Errorf("interaction: advertised Tools: %w", err)
	}
	if len(appliedSteerSignalIDs) > 0 {
		if err := validateSteerSignalIDs(appliedSteerSignalIDs); err != nil {
			return effectEnvelope{}, fmt.Errorf("interaction: applied steer SignalIDs: %w", err)
		}
	}
	cloned := request.Clone()
	if err := cloned.Validate(); err != nil {
		return effectEnvelope{}, fmt.Errorf("interaction: model request: %w", err)
	}
	return effectEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelCall: &modelCall{
			ModelCallSequence:     modelCallSequence,
			Request:               *cloned,
			AdvertisedToolNames:   slices.Clone(advertisedToolNames),
			AppliedSteerSignalIDs: slices.Clone(appliedSteerSignalIDs),
		},
	}, nil
}

func newToolBatchEffect(
	modelCallSequence uint32,
	firstToolCallIndex uint32,
	calls []chat.ToolCall,
	checkpoint *toolCheckpoint,
	inputResponse json.RawMessage,
) (effectEnvelope, error) {
	if modelCallSequence == 0 {
		return effectEnvelope{}, errors.New("interaction: model call sequence is required")
	}
	if len(calls) == 0 {
		return effectEnvelope{}, errors.New("interaction: Tool batch is empty")
	}
	if uint64(firstToolCallIndex)+uint64(len(calls)) > uint64(^uint32(0))+1 {
		return effectEnvelope{}, errors.New("interaction: Tool batch index range overflows")
	}
	cloned := append([]chat.ToolCall(nil), calls...)
	for index := range cloned {
		if err := cloned[index].Validate(); err != nil {
			return effectEnvelope{}, fmt.Errorf("interaction: tool batch call %d: %w", index, err)
		}
	}
	batch := &toolBatchCall{
		ModelCallSequence:  modelCallSequence,
		FirstToolCallIndex: firstToolCallIndex,
		Calls:              cloned,
	}
	if checkpoint != nil {
		if err := checkpoint.validate(cloned); err != nil {
			return effectEnvelope{}, err
		}
		response, err := checkpoint.InputRequest.inputRequest()
		if err != nil {
			return effectEnvelope{}, err
		}
		inputResponse, err = response.validateResponse(inputResponse)
		if err != nil {
			return effectEnvelope{}, err
		}
		clonedCheckpoint := checkpoint.clone()
		batch.Checkpoint = &clonedCheckpoint
		batch.InputResponse = inputResponse
	} else if len(inputResponse) != 0 {
		return effectEnvelope{}, errors.New("interaction: input response requires a tool checkpoint")
	}
	return effectEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolBatch:     batch,
	}, nil
}

func (envelope effectEnvelope) validate() error {
	if envelope.SchemaVersion != protocolSchemaVersion ||
		(envelope.Operation != operationModelCall && envelope.Operation != operationToolBatch) {
		return errors.New("interaction: unsupported effect protocol")
	}
	switch envelope.Operation {
	case operationModelCall:
		return envelope.validateModelCall()
	case operationToolBatch:
		return envelope.validateToolBatch()
	}
	return nil
}

func (envelope effectEnvelope) validateModelCall() error {
	if envelope.ModelCall == nil || envelope.ToolBatch != nil || envelope.ModelCall.ModelCallSequence == 0 {
		return errors.New("interaction: model_call effect has an invalid payload set")
	}
	if err := envelope.ModelCall.Request.Validate(); err != nil {
		return fmt.Errorf("interaction: model_call request: %w", err)
	}
	if err := validateAdvertisedToolNames(envelope.ModelCall.AdvertisedToolNames); err != nil {
		return fmt.Errorf("interaction: model_call advertised Tools: %w", err)
	}
	if len(envelope.ModelCall.AppliedSteerSignalIDs) > 0 {
		if err := validateSteerSignalIDs(envelope.ModelCall.AppliedSteerSignalIDs); err != nil {
			return fmt.Errorf("interaction: model_call applied steer SignalIDs: %w", err)
		}
	}
	return nil
}

func (envelope effectEnvelope) validateToolBatch() error {
	if envelope.ModelCall != nil || envelope.ToolBatch == nil ||
		envelope.ToolBatch.ModelCallSequence == 0 || len(envelope.ToolBatch.Calls) == 0 ||
		uint64(envelope.ToolBatch.FirstToolCallIndex)+uint64(len(envelope.ToolBatch.Calls)) > uint64(^uint32(0))+1 {
		return errors.New("interaction: tool_batch effect has an invalid payload set")
	}
	for index := range envelope.ToolBatch.Calls {
		if err := envelope.ToolBatch.Calls[index].Validate(); err != nil {
			return fmt.Errorf("interaction: tool_batch call %d: %w", index, err)
		}
	}
	if envelope.ToolBatch.Checkpoint == nil {
		if len(envelope.ToolBatch.InputResponse) != 0 {
			return errors.New("interaction: tool_batch input response requires a checkpoint")
		}
		return nil
	}
	if err := envelope.ToolBatch.Checkpoint.validate(envelope.ToolBatch.Calls); err != nil {
		return err
	}
	request, err := envelope.ToolBatch.Checkpoint.InputRequest.inputRequest()
	if err != nil {
		return err
	}
	_, err = request.validateResponse(envelope.ToolBatch.InputResponse)
	return err
}

func (envelope signalEnvelope) validate() error {
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() {
		return errors.New("interaction: unsupported signal protocol")
	}
	switch envelope.Operation {
	case operationModelCall:
		return envelope.validateModelResult()
	case operationToolBatch:
		return envelope.validateToolResult()
	case operationWaitOpened:
		return envelope.validateWaitOpened()
	case operationInputResponse:
		return envelope.validateInputResponse()
	case operationSteer:
		return envelope.validateSteer()
	}
	return nil
}

func (envelope signalEnvelope) validateModelResult() error {
	if envelope.ModelResult == nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
		return errors.New("interaction: model_result signal has an invalid payload set")
	}
	result := envelope.ModelResult
	modes := 0
	if result.Response != nil {
		modes++
	}
	if result.Error != "" {
		modes++
	}
	if result.HostError != "" {
		modes++
	}
	if modes != 1 {
		return errors.New("interaction: model_result requires exactly one response, provider error, or host error")
	}
	if result.Response != nil {
		if err := result.Response.Validate(); err != nil {
			return fmt.Errorf("interaction: model_result response: %w", err)
		}
	}
	return nil
}

func (envelope signalEnvelope) validateToolResult() error {
	if envelope.ModelResult != nil || envelope.ToolResult == nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
		return errors.New("interaction: tool_result signal has an invalid payload set")
	}
	result := envelope.ToolResult
	modes := 0
	if len(result.Results) != 0 {
		modes++
	}
	if result.Checkpoint != nil {
		modes++
	}
	if result.HostError != "" {
		modes++
	}
	if modes != 1 {
		return errors.New("interaction: tool_result requires complete results, a checkpoint, or a host error")
	}
	if result.HostError != "" {
		if result.Direct || len(result.AdvertisedToolNames) != 0 {
			return errors.New("interaction: failed tool_result must carry only its host error")
		}
	} else if result.Checkpoint != nil {
		if result.Direct || len(result.AdvertisedToolNames) != 0 {
			return errors.New("interaction: paused tool_result must carry only its checkpoint")
		}
		if _, err := result.Checkpoint.InputRequest.inputRequest(); err != nil {
			return err
		}
	} else {
		for index := range result.Results {
			if err := result.Results[index].Validate(); err != nil {
				return fmt.Errorf("interaction: tool_result %d: %w", index, err)
			}
		}
	}
	if err := validateAdvertisedToolNames(result.AdvertisedToolNames); err != nil {
		return fmt.Errorf("interaction: tool_result advertised Tools: %w", err)
	}
	return nil
}

func (envelope signalEnvelope) validateWaitOpened() error {
	if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened == nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
		return errors.New("interaction: wait_opened signal has an invalid payload set")
	}
	_, err := envelope.WaitOpened.inputRequest()
	return err
}

func (envelope signalEnvelope) validateInputResponse() error {
	if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) == 0 || envelope.Steer != nil {
		return errors.New("interaction: input_response signal has an invalid payload set")
	}
	if _, err := canonicalJSON(envelope.InputResponse); err != nil {
		return fmt.Errorf("interaction: input_response: %w", err)
	}
	return nil
}

func (envelope signalEnvelope) validateSteer() error {
	if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer == nil {
		return errors.New("interaction: steer signal has an invalid payload set")
	}
	return validateSteeringMessages(envelope.Steer.Messages)
}

func (checkpoint toolCheckpoint) clone() toolCheckpoint {
	cloned := checkpoint
	cloned.CompletedResults = append([]chat.ToolResult(nil), checkpoint.CompletedResults...)
	cloned.AdvertisedToolNames = slices.Clone(checkpoint.AdvertisedToolNames)
	cloned.InputRequest.Prompt = append(json.RawMessage(nil), checkpoint.InputRequest.Prompt...)
	cloned.InputRequest.ResponseSchema = append(json.RawMessage(nil), checkpoint.InputRequest.ResponseSchema...)
	cloned.InputRequest.ContinuationState = append(json.RawMessage(nil), checkpoint.InputRequest.ContinuationState...)
	return cloned
}

func (checkpoint toolCheckpoint) validate(calls []chat.ToolCall) error {
	if checkpoint.PauseCount == 0 || checkpoint.NextToolCallIndex != uint32(len(checkpoint.CompletedResults)) ||
		int(checkpoint.NextToolCallIndex) >= len(calls) {
		return errors.New("interaction: invalid tool checkpoint position")
	}
	if _, err := checkpoint.InputRequest.inputRequest(); err != nil {
		return fmt.Errorf("interaction: tool checkpoint input: %w", err)
	}
	if err := validateAdvertisedToolNames(checkpoint.AdvertisedToolNames); err != nil {
		return fmt.Errorf("interaction: tool checkpoint advertised Tools: %w", err)
	}
	for index := range checkpoint.CompletedResults {
		result := checkpoint.CompletedResults[index]
		if err := result.Validate(); err != nil {
			return fmt.Errorf("interaction: tool checkpoint result %d: %w", index, err)
		}
		if result.ID != calls[index].ID || result.Name != calls[index].Name {
			return fmt.Errorf("interaction: tool checkpoint result %d does not match call %q", index, calls[index].ID)
		}
	}
	return nil
}

func encodeProtocol(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("interaction: encode protocol payload: %w", err)
	}
	return payload, nil
}

func decodeEffect(data json.RawMessage) (effectEnvelope, error) {
	var envelope effectEnvelope
	if err := decodeStrict(data, &envelope); err != nil {
		return effectEnvelope{}, fmt.Errorf("interaction: decode effect: %w", err)
	}
	if err := envelope.validate(); err != nil {
		return effectEnvelope{}, err
	}
	return envelope, nil
}

func decodeSignal(data json.RawMessage) (signalEnvelope, error) {
	var envelope signalEnvelope
	if err := decodeStrict(data, &envelope); err != nil {
		return signalEnvelope{}, fmt.Errorf("interaction: decode signal: %w", err)
	}
	if err := envelope.validate(); err != nil {
		return signalEnvelope{}, err
	}
	return envelope, nil
}
