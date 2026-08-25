package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	googlejsonschema "github.com/google/jsonschema-go/jsonschema"
	validationjsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const maxSchemaBytes = 1 << 20

const schemaResourceURL = "urn:lynx:agent:schema"

// ErrInvalidSchema reports malformed, unsupported, or unresolved JSON Schema.
var ErrInvalidSchema = errors.New("agent: invalid schema")

// Schema is an immutable, resolved JSON Schema. It is safe for concurrent
// validation after construction. Its zero value is invalid.
type Schema struct {
	data     json.RawMessage
	compiled *validationjsonschema.Schema
}

// ParseSchema validates and resolves one JSON Schema.
func ParseSchema(data json.RawMessage) (Schema, error) {
	normalized, err := wireJSON.normalize(data, maxSchemaBytes)
	if err != nil {
		return Schema{}, fmt.Errorf("%w: %w", ErrInvalidSchema, err)
	}
	document, err := validationjsonschema.UnmarshalJSON(bytes.NewReader(normalized))
	if err != nil {
		return Schema{}, fmt.Errorf("%w: decode: %w", ErrInvalidSchema, err)
	}
	compiler := validationjsonschema.NewCompiler()
	compiler.DefaultDraft(validationjsonschema.Draft2020)
	compiler.UseLoader(validationjsonschema.SchemeURLLoader{})
	if err := compiler.AddResource(schemaResourceURL, document); err != nil {
		return Schema{}, fmt.Errorf("%w: add resource: %w", ErrInvalidSchema, err)
	}
	compiled, err := compiler.Compile(schemaResourceURL)
	if err != nil {
		return Schema{}, fmt.Errorf("%w: compile: %w", ErrInvalidSchema, err)
	}
	return Schema{data: normalized, compiled: compiled}, nil
}

// SchemaFor derives and resolves a JSON Schema for T.
func SchemaFor[T any]() (Schema, error) {
	definition, err := googlejsonschema.For[T](schemaForOptions())
	if err != nil {
		return Schema{}, fmt.Errorf("%w: derive: %w", ErrInvalidSchema, err)
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return Schema{}, fmt.Errorf("%w: encode derived schema: %w", ErrInvalidSchema, err)
	}
	return ParseSchema(data)
}

func schemaForOptions() *googlejsonschema.ForOptions {
	return &googlejsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*googlejsonschema.Schema{
			// RawMessage already contains one JSON value, so its Go []byte
			// representation must not constrain the value's JSON kind.
			reflect.TypeFor[json.RawMessage](): {},
			// encoding/json represents byte slices as base64 strings (or null),
			// not as JSON arrays of integers.
			reflect.TypeFor[[]byte](): {
				Types:           []string{"null", "string"},
				ContentEncoding: "base64",
			},
		},
	}
}

// JSON returns an independently owned JSON representation.
func (s Schema) JSON() json.RawMessage { return bytes.Clone(s.data) }

// Valid reports whether the Schema was parsed and resolved successfully.
func (s Schema) Valid() bool { return len(s.data) > 0 && s.compiled != nil }

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
	value, err := validationjsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if err := s.compiled.Validate(value); err != nil {
		return err
	}
	return nil
}

func (s Schema) clone() Schema {
	return Schema{data: bytes.Clone(s.data), compiled: s.compiled}
}

// MarshalJSON returns the validated canonical JSON Schema document.
func (s Schema) MarshalJSON() ([]byte, error) {
	if !s.Valid() {
		return nil, ErrInvalidSchema
	}
	return bytes.Clone(s.data), nil
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
