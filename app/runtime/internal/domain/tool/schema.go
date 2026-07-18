package tool

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidSchema reports a registered tool schema that is not one complete
// JSON object.
var ErrInvalidSchema = errors.New("tool: invalid input schema")

// Schema is an immutable, canonical JSON Schema object advertised by a
// registered tool. The zero value is the empty schema object.
type Schema struct {
	raw string
}

// ParseSchema validates and owns a JSON-encoded schema. JSON Schema permits an
// empty object, but not null, scalars, arrays, malformed JSON, or multiple
// documents at this boundary.
func ParseSchema(data []byte) (Schema, error) {
	var value map[string]any
	if err := decodeValue(data, &value); err != nil {
		return Schema{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	if value == nil {
		return Schema{}, fmt.Errorf("%w: expected an object", ErrInvalidSchema)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Schema{}, fmt.Errorf("%w: normalize: %w", ErrInvalidSchema, err)
	}
	if string(encoded) == "{}" {
		return Schema{}, nil
	}
	return Schema{raw: string(encoded)}, nil
}

// String returns the canonical schema spelling.
func (s Schema) String() string {
	if s.raw == "" {
		return "{}"
	}
	return s.raw
}

// Map returns a recursively ownership-isolated wire projection while
// preserving JSON numbers exactly.
func (s Schema) Map() map[string]any {
	var value map[string]any
	if err := decodeValue([]byte(s.String()), &value); err != nil {
		panic(fmt.Sprintf("tool: corrupt Schema invariant: %v", err))
	}
	return value
}
