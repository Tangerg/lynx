package chat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

var ErrInvalidToolDefinition = errors.New("chat: invalid tool definition")

var toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ToolDefinition is the serializable description exposed to a model. Tool
// execution belongs to package tool and is deliberately absent here.
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

// Validate verifies the provider-compatible tool name and object input schema.
func (d ToolDefinition) Validate() error {
	if !toolNamePattern.MatchString(d.Name) {
		return fmt.Errorf("%w: name must match %s", ErrInvalidToolDefinition, toolNamePattern)
	}
	var schema map[string]json.RawMessage
	if len(d.InputSchema) == 0 {
		return fmt.Errorf("%w: missing input schema", ErrInvalidToolDefinition)
	}
	if err := json.Unmarshal(d.InputSchema, &schema); err != nil || schema == nil {
		return fmt.Errorf("%w: input schema must be a JSON object", ErrInvalidToolDefinition)
	}
	var schemaType string
	if err := json.Unmarshal(schema["type"], &schemaType); err != nil || schemaType != "object" {
		return fmt.Errorf("%w: input schema type must be %q", ErrInvalidToolDefinition, "object")
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
