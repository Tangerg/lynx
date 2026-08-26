package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidInputSchema reports an MCP tool input schema that cannot represent
// an object-shaped argument document.
var ErrInvalidInputSchema = errors.New("mcp: invalid tool input schema")

const emptyInputSchema = `{"type":"object"}`

// InputSchema is an immutable MCP tool input schema. MCP tools accept JSON
// objects, so the value rejects malformed, non-object, and non-object-typed
// schemas at the connection boundary instead of leaking an untyped map through
// the runtime.
//
// The zero value is the schema for an unconstrained JSON object.
type InputSchema struct {
	object string
}

// NewInputSchema converts a supplied MCP schema into an owned,
// canonical value.
func NewInputSchema(value any) (InputSchema, error) {
	if value == nil {
		return InputSchema{}, fmt.Errorf("%w: schema is required", ErrInvalidInputSchema)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return InputSchema{}, fmt.Errorf("%w: encode schema: %w", ErrInvalidInputSchema, err)
	}
	return ParseInputSchema(data)
}

// ParseInputSchema validates and owns a JSON-encoded MCP input schema.
func ParseInputSchema(data []byte) (InputSchema, error) {
	if len(data) > MaxRemoteToolInputSchemaBytes {
		return InputSchema{}, fmt.Errorf(
			"%w: schema uses %d bytes, maximum %d",
			ErrInvalidInputSchema,
			len(data),
			MaxRemoteToolInputSchemaBytes,
		)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return InputSchema{}, fmt.Errorf("%w: empty JSON document", ErrInvalidInputSchema)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return InputSchema{}, fmt.Errorf("%w: decode schema: %w", ErrInvalidInputSchema, err)
	}
	if object == nil {
		return InputSchema{}, fmt.Errorf("%w: expected a JSON object", ErrInvalidInputSchema)
	}

	var schemaType string
	typeJSON, ok := object["type"]
	if !ok {
		return InputSchema{}, fmt.Errorf("%w: missing object type", ErrInvalidInputSchema)
	}
	if err := json.Unmarshal(typeJSON, &schemaType); err != nil {
		return InputSchema{}, fmt.Errorf("%w: type must be the string %q: %w", ErrInvalidInputSchema, "object", err)
	}
	if schemaType != "object" {
		return InputSchema{}, fmt.Errorf("%w: type must be %q", ErrInvalidInputSchema, "object")
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return InputSchema{}, fmt.Errorf("%w: normalize schema: %w", ErrInvalidInputSchema, err)
	}
	if len(normalized) > MaxRemoteToolInputSchemaBytes {
		return InputSchema{}, fmt.Errorf(
			"%w: normalized schema uses %d bytes, maximum %d",
			ErrInvalidInputSchema,
			len(normalized),
			MaxRemoteToolInputSchemaBytes,
		)
	}
	if string(normalized) == emptyInputSchema {
		return InputSchema{}, nil
	}
	return InputSchema{object: string(normalized)}, nil
}

// JSON returns an ownership-isolated canonical representation.
func (i InputSchema) JSON() []byte {
	return []byte(i.String())
}

// String returns the canonical JSON schema.
func (i InputSchema) String() string {
	if i.object == "" {
		return emptyInputSchema
	}
	return i.object
}

// Map projects the immutable schema to a fresh MCP tool contract. Each call
// returns a fresh object graph and preserves JSON numbers exactly.
func (i InputSchema) Map() map[string]any {
	decoder := json.NewDecoder(bytes.NewBufferString(i.String()))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		panic(fmt.Sprintf("mcp: corrupt input schema invariant: %v", err))
	}
	return object
}
