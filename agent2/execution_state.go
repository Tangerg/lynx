package agent2

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

// NewExecutionState validates and takes an immutable snapshot of a Strategy
// state payload. kind identifies the owning Strategy, not a global factory.
func NewExecutionState(kind string, schemaVersion uint16, payload json.RawMessage) (ExecutionState, error) {
	if !validQualifiedName(kind) {
		return ExecutionState{}, fmt.Errorf("%w: kind must be a lowercase qualified name", ErrInvalidExecutionState)
	}
	if schemaVersion == 0 {
		return ExecutionState{}, fmt.Errorf("%w: schema version must be greater than zero", ErrInvalidExecutionState)
	}
	normalized, err := normalizeJSON(payload, maxWireBytes)
	if err != nil {
		return ExecutionState{}, fmt.Errorf("%w: payload: %w", ErrInvalidExecutionState, err)
	}
	return ExecutionState{kind: kind, schemaVersion: schemaVersion, payload: normalized}, nil
}

// Kind returns the Strategy that exclusively interprets Payload.
func (s ExecutionState) Kind() string { return s.kind }

// SchemaVersion returns the Strategy-owned payload schema version.
func (s ExecutionState) SchemaVersion() uint16 { return s.schemaVersion }

// Payload returns an independently owned copy of the opaque Strategy state.
func (s ExecutionState) Payload() json.RawMessage { return bytes.Clone(s.payload) }

// Valid reports whether the state was created through its validated boundary.
func (s ExecutionState) Valid() bool {
	return validQualifiedName(s.kind) && s.schemaVersion > 0 && len(s.payload) > 0
}

func (s ExecutionState) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidExecutionState
	}
	return json.Marshal(executionStateWire{
		Kind:          s.kind,
		SchemaVersion: s.schemaVersion,
		Payload:       s.payload,
	})
}

func (s *ExecutionState) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidExecutionState)
	}
	var wire executionStateWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidExecutionState, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidExecutionState, err)
	}
	value, err := NewExecutionState(wire.Kind, wire.SchemaVersion, wire.Payload)
	if err != nil {
		return err
	}
	*s = value
	return nil
}

type executionStateWire struct {
	Kind          string          `json:"kind"`
	SchemaVersion uint16          `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}
