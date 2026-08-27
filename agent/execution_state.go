package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidExecutionState = errors.New("agent: invalid execution state")

// ExecutionState is an immutable, versioned envelope owned by one Execution
// Strategy. The Engine persists and returns Payload without interpreting it.
// Its zero value is invalid.
type ExecutionState struct {
	kind          string
	schemaVersion uint16
	payload       json.RawMessage
}

func NewExecutionState(kind string, schemaVersion uint16, payload json.RawMessage) (ExecutionState, error) {
	if !validQualifiedName(kind) {
		return ExecutionState{}, fmt.Errorf("%w: kind must be a lowercase qualified name", ErrInvalidExecutionState)
	}
	if schemaVersion == 0 {
		return ExecutionState{}, fmt.Errorf("%w: schema version must be greater than zero", ErrInvalidExecutionState)
	}
	normalized, err := wireJSON.normalize(payload, maxWireBytes)
	if err != nil {
		return ExecutionState{}, fmt.Errorf("%w: payload: %w", ErrInvalidExecutionState, err)
	}
	return ExecutionState{kind: kind, schemaVersion: schemaVersion, payload: normalized}, nil
}

// Kind returns the Strategy that exclusively interprets Payload.
func (e ExecutionState) Kind() string { return e.kind }

// SchemaVersion returns the Strategy-owned payload schema version.
func (e ExecutionState) SchemaVersion() uint16 { return e.schemaVersion }

// Payload returns an independently owned copy of the opaque Strategy state.
func (e ExecutionState) Payload() json.RawMessage { return bytes.Clone(e.payload) }

func (e ExecutionState) Valid() bool {
	return validQualifiedName(e.kind) && e.schemaVersion > 0 && len(e.payload) > 0
}

func (e ExecutionState) clone() ExecutionState {
	return ExecutionState{
		kind: e.kind, schemaVersion: e.schemaVersion, payload: bytes.Clone(e.payload),
	}
}

func (e ExecutionState) MarshalJSON() ([]byte, error) {
	if !e.Valid() {
		return nil, ErrInvalidExecutionState
	}
	return json.Marshal(executionStateWire{
		Kind:          e.kind,
		SchemaVersion: e.schemaVersion,
		Payload:       e.payload,
	})
}

func (e *ExecutionState) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidExecutionState)
	}
	wire, err := wireJSON.decode[executionStateWire](data)
	if err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidExecutionState, err)
	}
	value, err := NewExecutionState(wire.Kind, wire.SchemaVersion, wire.Payload)
	if err != nil {
		return err
	}
	*e = value
	return nil
}

type executionStateWire struct {
	Kind          string          `json:"kind"`
	SchemaVersion uint16          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}
