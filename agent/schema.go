package agent

import (
	"encoding/json"
	"errors"
	"fmt"

	corejsonschema "github.com/Tangerg/scope/core/jsonschema"
)

// ErrInvalidSchema reports malformed, unsupported, or unresolved JSON Schema.
var ErrInvalidSchema = errors.New("agent: invalid schema")

// Schema is an immutable, resolved JSON Schema used by Framework input and
// output contracts. Its zero value is invalid.
type Schema struct {
	contract corejsonschema.Schema
}

// ParseSchema validates and resolves one JSON Schema.
func ParseSchema(data json.RawMessage) (Schema, error) {
	contract, err := corejsonschema.Parse(data)
	if err != nil {
		return Schema{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	return Schema{contract: contract}, nil
}

// SchemaFor derives and resolves a JSON Schema for T.
func SchemaFor[T any]() (Schema, error) {
	contract, err := corejsonschema.For[T]()
	if err != nil {
		return Schema{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	return Schema{contract: contract}, nil
}

// JSON returns an independently owned JSON representation.
func (s Schema) JSON() json.RawMessage { return s.contract.JSON() }

// Valid reports whether the Schema was parsed and resolved successfully.
func (s Schema) Valid() bool { return s.contract.Valid() }

// ValidateInput validates input against the schema.
func (s Schema) ValidateInput(input Input) error {
	if err := s.validate(input.data); err != nil {
		return fmt.Errorf("%w: schema validation: %w", ErrInvalidInput, err)
	}
	return nil
}

// ValidateOutput validates output against the schema.
func (s Schema) ValidateOutput(output Output) error {
	if err := s.validate(output.data); err != nil {
		return fmt.Errorf("%w: schema validation: %w", ErrInvalidOutput, err)
	}
	return nil
}

func (s Schema) validate(data []byte) error {
	if !s.Valid() {
		return ErrInvalidSchema
	}
	return s.contract.Validate(data)
}

func (s Schema) clone() Schema { return s }

// MarshalJSON returns the validated canonical JSON Schema document.
func (s Schema) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidSchema
	}
	return s.contract.JSON(), nil
}

// UnmarshalJSON replaces s with a parsed and resolved JSON Schema.
func (s *Schema) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidSchema)
	}
	value, err := ParseSchema(data)
	if err != nil {
		return err
	}
	*s = value
	return nil
}
