package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidToolDefinition = errors.New("chat: invalid tool definition")

// ToolDefinition is the serializable description exposed to a model. Tool
// execution belongs to the tools module and is deliberately absent here.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Clone returns an independent copy of d.
func (d ToolDefinition) Clone() ToolDefinition {
	d.InputSchema = bytes.Clone(d.InputSchema)
	return d
}

// Validate verifies the tool name and JSON-object input schema.
func (d ToolDefinition) Validate() error {
	if d.Name == "" || strings.IndexFunc(d.Name, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' }) >= 0 {
		return fmt.Errorf("%w: name must be non-empty and contain no whitespace", ErrInvalidToolDefinition)
	}
	var schema map[string]json.RawMessage
	if len(d.InputSchema) == 0 {
		return fmt.Errorf("%w: missing input schema", ErrInvalidToolDefinition)
	}
	if err := json.Unmarshal(d.InputSchema, &schema); err != nil || schema == nil {
		return fmt.Errorf("%w: input schema must be a JSON object", ErrInvalidToolDefinition)
	}
	return nil
}

// MarshalJSON validates d before writing its wire representation.
func (d ToolDefinition) MarshalJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	type wireToolDefinition ToolDefinition
	return json.Marshal(wireToolDefinition(d))
}

// UnmarshalJSON decodes and validates a definition before replacing the
// receiver.
func (d *ToolDefinition) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidToolDefinition)
	}
	type wireToolDefinition ToolDefinition
	var decoded wireToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidToolDefinition, err)
	}
	candidate := ToolDefinition(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*d = candidate
	return nil
}
