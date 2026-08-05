package interaction

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

const protocolSchemaVersion uint16 = 1

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
	Request chat.Request `json:"request"`
}

type toolBatchCall struct {
	Calls         []chat.ToolCall `json:"calls"`
	Checkpoint    *toolCheckpoint `json:"checkpoint,omitempty"`
	InputResponse json.RawMessage `json:"input_response,omitempty"`
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
	Response *chat.Response `json:"response,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type steerInput struct {
	Messages []chat.Message `json:"messages"`
}

type toolBatchResult struct {
	Results    []chat.ToolResult `json:"results"`
	Direct     bool              `json:"direct"`
	Checkpoint *toolCheckpoint   `json:"checkpoint,omitempty"`
}

type toolCheckpoint struct {
	Completed []chat.ToolResult `json:"completed"`
	Next      uint32            `json:"next"`
	Pauses    uint32            `json:"pauses"`
	Input     inputRequestWire  `json:"input"`
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

func newModelEffect(request *chat.Request) (effectEnvelope, error) {
	if request == nil {
		return effectEnvelope{}, errors.New("interaction: model request is nil")
	}
	cloned := request.Clone()
	if err := cloned.Validate(); err != nil {
		return effectEnvelope{}, fmt.Errorf("interaction: model request: %w", err)
	}
	return effectEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationModelCall,
		ModelCall:     &modelCall{Request: *cloned},
	}, nil
}

func newToolBatchEffect(
	calls []chat.ToolCall,
	checkpoint *toolCheckpoint,
	inputResponse json.RawMessage,
) (effectEnvelope, error) {
	if len(calls) == 0 {
		return effectEnvelope{}, errors.New("interaction: tool batch is empty")
	}
	cloned := append([]chat.ToolCall(nil), calls...)
	for index := range cloned {
		if err := cloned[index].Validate(); err != nil {
			return effectEnvelope{}, fmt.Errorf("interaction: tool batch call %d: %w", index, err)
		}
	}
	batch := &toolBatchCall{Calls: cloned}
	if checkpoint != nil {
		if err := checkpoint.validate(cloned); err != nil {
			return effectEnvelope{}, err
		}
		response, err := checkpoint.Input.inputRequest()
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
		if envelope.ModelCall == nil || envelope.ToolBatch != nil {
			return errors.New("interaction: model_call effect has an invalid payload set")
		}
		if err := envelope.ModelCall.Request.Validate(); err != nil {
			return fmt.Errorf("interaction: model_call request: %w", err)
		}
	case operationToolBatch:
		if envelope.ModelCall != nil || envelope.ToolBatch == nil || len(envelope.ToolBatch.Calls) == 0 {
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
		} else {
			if err := envelope.ToolBatch.Checkpoint.validate(envelope.ToolBatch.Calls); err != nil {
				return err
			}
			request, err := envelope.ToolBatch.Checkpoint.Input.inputRequest()
			if err != nil {
				return err
			}
			if _, err := request.validateResponse(envelope.ToolBatch.InputResponse); err != nil {
				return err
			}
		}
	}
	return nil
}

func (envelope signalEnvelope) validate() error {
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() {
		return errors.New("interaction: unsupported signal protocol")
	}
	switch envelope.Operation {
	case operationModelCall:
		if envelope.ModelResult == nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
			return errors.New("interaction: model_result signal has an invalid payload set")
		}
		result := envelope.ModelResult
		if (result.Response == nil) == (result.Error == "") {
			return errors.New("interaction: model_result requires exactly one response or error")
		}
		if result.Response != nil {
			if err := result.Response.Validate(); err != nil {
				return fmt.Errorf("interaction: model_result response: %w", err)
			}
		}
	case operationToolBatch:
		if envelope.ModelResult != nil || envelope.ToolResult == nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
			return errors.New("interaction: tool_result signal has an invalid payload set")
		}
		if (len(envelope.ToolResult.Results) == 0) == (envelope.ToolResult.Checkpoint == nil) {
			return errors.New("interaction: tool_result requires complete results or a checkpoint")
		}
		if envelope.ToolResult.Checkpoint != nil {
			if envelope.ToolResult.Direct {
				return errors.New("interaction: paused tool_result cannot be direct")
			}
			if _, err := envelope.ToolResult.Checkpoint.Input.inputRequest(); err != nil {
				return err
			}
		} else {
			for index := range envelope.ToolResult.Results {
				if err := envelope.ToolResult.Results[index].Validate(); err != nil {
					return fmt.Errorf("interaction: tool_result %d: %w", index, err)
				}
			}
		}
	case operationWaitOpened:
		if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened == nil || len(envelope.InputResponse) != 0 || envelope.Steer != nil {
			return errors.New("interaction: wait_opened signal has an invalid payload set")
		}
		if _, err := envelope.WaitOpened.inputRequest(); err != nil {
			return err
		}
	case operationInputResponse:
		if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) == 0 || envelope.Steer != nil {
			return errors.New("interaction: input_response signal has an invalid payload set")
		}
		if _, err := canonicalJSON(envelope.InputResponse); err != nil {
			return fmt.Errorf("interaction: input_response: %w", err)
		}
	case operationSteer:
		if envelope.ModelResult != nil || envelope.ToolResult != nil || envelope.WaitOpened != nil || len(envelope.InputResponse) != 0 || envelope.Steer == nil {
			return errors.New("interaction: steer signal has an invalid payload set")
		}
		if err := validateSteeringMessages(envelope.Steer.Messages); err != nil {
			return err
		}
	}
	return nil
}

func (checkpoint toolCheckpoint) clone() toolCheckpoint {
	cloned := checkpoint
	cloned.Completed = append([]chat.ToolResult(nil), checkpoint.Completed...)
	cloned.Input.Prompt = append(json.RawMessage(nil), checkpoint.Input.Prompt...)
	cloned.Input.ResponseSchema = append(json.RawMessage(nil), checkpoint.Input.ResponseSchema...)
	cloned.Input.ContinuationState = append(json.RawMessage(nil), checkpoint.Input.ContinuationState...)
	return cloned
}

func (checkpoint toolCheckpoint) validate(calls []chat.ToolCall) error {
	if checkpoint.Pauses == 0 || checkpoint.Next != uint32(len(checkpoint.Completed)) ||
		int(checkpoint.Next) >= len(calls) {
		return errors.New("interaction: invalid tool checkpoint position")
	}
	if _, err := checkpoint.Input.inputRequest(); err != nil {
		return fmt.Errorf("interaction: tool checkpoint input: %w", err)
	}
	for index := range checkpoint.Completed {
		result := checkpoint.Completed[index]
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
