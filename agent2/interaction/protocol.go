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
	operationModelCall operation = "model_call"
	operationToolBatch operation = "tool_batch"
)

func (value operation) valid() bool {
	return value == operationModelCall || value == operationToolBatch
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
	Calls []chat.ToolCall `json:"calls"`
}

type signalEnvelope struct {
	SchemaVersion uint16           `json:"schema_version"`
	Operation     operation        `json:"operation"`
	ModelResult   *modelCallResult `json:"model_result,omitempty"`
	ToolResult    *toolBatchResult `json:"tool_result,omitempty"`
}

type modelCallResult struct {
	Response *chat.Response `json:"response,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type toolBatchResult struct {
	Results []chat.ToolResult `json:"results"`
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

func newToolBatchEffect(calls []chat.ToolCall) (effectEnvelope, error) {
	if len(calls) == 0 {
		return effectEnvelope{}, errors.New("interaction: tool batch is empty")
	}
	cloned := append([]chat.ToolCall(nil), calls...)
	for index := range cloned {
		if err := cloned[index].Validate(); err != nil {
			return effectEnvelope{}, fmt.Errorf("interaction: tool batch call %d: %w", index, err)
		}
	}
	return effectEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationToolBatch,
		ToolBatch:     &toolBatchCall{Calls: cloned},
	}, nil
}

func (envelope effectEnvelope) validate() error {
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() {
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
	}
	return nil
}

func (envelope signalEnvelope) validate() error {
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() {
		return errors.New("interaction: unsupported signal protocol")
	}
	switch envelope.Operation {
	case operationModelCall:
		if envelope.ModelResult == nil || envelope.ToolResult != nil {
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
		if envelope.ModelResult != nil || envelope.ToolResult == nil || len(envelope.ToolResult.Results) == 0 {
			return errors.New("interaction: tool_result signal has an invalid payload set")
		}
		for index := range envelope.ToolResult.Results {
			if err := envelope.ToolResult.Results[index].Validate(); err != nil {
				return fmt.Errorf("interaction: tool_result %d: %w", index, err)
			}
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
