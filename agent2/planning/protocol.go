package planning

import (
	"encoding/json"
	"errors"
	"fmt"

	agent "github.com/Tangerg/lynx/agent2"
)

const protocolSchemaVersion uint16 = 1

type operation string

const (
	operationObserve operation = "observe"
	operationAction  operation = "action"
)

func (value operation) valid() bool { return value == operationObserve || value == operationAction }

type effectEnvelope struct {
	SchemaVersion uint16      `json:"schema_version"`
	Operation     operation   `json:"operation"`
	Input         agent.Input `json:"input"`
	Action        *actionCall `json:"action,omitempty"`
}

type actionCall struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	WorldState  WorldState `json:"world_state"`
}

type signalEnvelope struct {
	SchemaVersion uint16             `json:"schema_version"`
	Operation     operation          `json:"operation"`
	Observation   *observationResult `json:"observation,omitempty"`
	Action        *actionResultWire  `json:"action,omitempty"`
}

type observationResult struct {
	WorldState *WorldState `json:"world_state,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type actionResultWire struct {
	Succeeded  bool   `json:"succeeded"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func newObservationEffect(input agent.Input) (agent.Effect, error) {
	if !input.Valid() {
		return agent.Effect{}, ErrInvalidProtocol
	}
	payload, err := encodeProtocol(effectEnvelope{
		SchemaVersion: protocolSchemaVersion, Operation: operationObserve, Input: input,
	})
	if err != nil {
		return agent.Effect{}, err
	}
	return agent.NewDispatcherEffect(payload)
}

func newActionEffect(input agent.Input, binding ActionBinding, state WorldState) (agent.Effect, error) {
	if !input.Valid() || !binding.Valid() || binding.target != bindingTargetDispatcher || !state.Valid() ||
		!binding.action.Applicable(state) {
		return agent.Effect{}, ErrInvalidProtocol
	}
	payload, err := encodeProtocol(effectEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationAction,
		Input:         input,
		Action: &actionCall{
			Name: binding.action.name, Description: binding.action.description, WorldState: state,
		},
	})
	if err != nil {
		return agent.Effect{}, err
	}
	return agent.NewDispatcherEffect(payload, binding.required.Values()...)
}

func decodeEffect(payload json.RawMessage) (effectEnvelope, error) {
	var envelope effectEnvelope
	if err := decodeStrict(payload, &envelope); err != nil {
		return effectEnvelope{}, fmt.Errorf("%w: decode Effect: %w", ErrInvalidProtocol, err)
	}
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() || !envelope.Input.Valid() {
		return effectEnvelope{}, ErrInvalidProtocol
	}
	switch envelope.Operation {
	case operationObserve:
		if envelope.Action != nil {
			return effectEnvelope{}, ErrInvalidProtocol
		}
	case operationAction:
		if envelope.Action == nil || !validName(envelope.Action.Name) ||
			!validDescription(envelope.Action.Description) || !envelope.Action.WorldState.Valid() {
			return effectEnvelope{}, ErrInvalidProtocol
		}
	}
	return envelope, nil
}

func observationSignal(state WorldState, cause error) (json.RawMessage, error) {
	result := &observationResult{}
	if cause != nil {
		result.Error = diagnostic(cause.Error())
	} else {
		if !state.Valid() {
			return nil, ErrInvalidProtocol
		}
		cloned := state
		result.WorldState = &cloned
	}
	return encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion, Operation: operationObserve, Observation: result,
	})
}

func actionSignal(result ActionResult) (json.RawMessage, error) {
	if !result.Valid() {
		return nil, ErrInvalidProtocol
	}
	return encodeProtocol(signalEnvelope{
		SchemaVersion: protocolSchemaVersion,
		Operation:     operationAction,
		Action: &actionResultWire{
			Succeeded: result.Succeeded(), Diagnostic: result.Diagnostic(),
		},
	})
}

func decodeSignal(payload json.RawMessage) (signalEnvelope, error) {
	var envelope signalEnvelope
	if err := decodeStrict(payload, &envelope); err != nil {
		return signalEnvelope{}, fmt.Errorf("%w: decode Signal: %w", ErrInvalidProtocol, err)
	}
	if envelope.SchemaVersion != protocolSchemaVersion || !envelope.Operation.valid() {
		return signalEnvelope{}, ErrInvalidProtocol
	}
	switch envelope.Operation {
	case operationObserve:
		if envelope.Observation == nil || envelope.Action != nil ||
			(envelope.Observation.WorldState == nil) == (envelope.Observation.Error == "") {
			return signalEnvelope{}, ErrInvalidProtocol
		}
		if envelope.Observation.WorldState != nil && !envelope.Observation.WorldState.Valid() ||
			envelope.Observation.Error != "" && diagnostic(envelope.Observation.Error) != envelope.Observation.Error {
			return signalEnvelope{}, ErrInvalidProtocol
		}
	case operationAction:
		if envelope.Action == nil || envelope.Observation != nil {
			return signalEnvelope{}, ErrInvalidProtocol
		}
		if envelope.Action.Succeeded && envelope.Action.Diagnostic != "" ||
			!envelope.Action.Succeeded && (envelope.Action.Diagnostic == "" ||
				diagnostic(envelope.Action.Diagnostic) != envelope.Action.Diagnostic) {
			return signalEnvelope{}, ErrInvalidProtocol
		}
	}
	return envelope, nil
}

func encodeProtocol(value any) (json.RawMessage, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: encode: %w", ErrInvalidProtocol, err)
	}
	return payload, nil
}

func oneSignal(signals []agent.Signal) (agent.Signal, error) {
	if len(signals) != 1 || !signals[0].Valid() {
		return agent.Signal{}, errors.New("planning: exactly one valid settlement Signal is required")
	}
	if _, addressed := signals[0].WaitID(); addressed {
		return agent.Signal{}, errors.New("planning: dispatcher settlement Signal must not address a wait")
	}
	return signals[0], nil
}
