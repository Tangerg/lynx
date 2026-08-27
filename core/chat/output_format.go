package chat

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	corejsonschema "github.com/Tangerg/scope/core/jsonschema"
)

// ErrInvalidOutputFormat reports a malformed output-format contract.
var ErrInvalidOutputFormat = errors.New("chat: invalid output format")

// OutputFormatType identifies the representation requested for a chat result.
// Provider adapters map the format to a native control when available and may
// fall back to equivalent prompt instructions otherwise.
type OutputFormatType string

const (
	OutputFormatText       OutputFormatType = "text"
	OutputFormatJSON       OutputFormatType = "json"
	OutputFormatJSONSchema OutputFormatType = "json_schema"
)

// OutputFormat is the provider-neutral representation contract for one model
// result. Name, Description, and Schema belong only to OutputFormatJSONSchema.
type OutputFormat struct {
	Type        OutputFormatType `json:"type"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Schema      json.RawMessage  `json:"schema,omitempty"`
}

// NewOutputFormat builds a text or unconstrained JSON output-format contract.
func NewOutputFormat(formatType OutputFormatType) (OutputFormat, error) {
	format := OutputFormat{Type: formatType}
	if err := format.Validate(); err != nil {
		return OutputFormat{}, fmt.Errorf("chat.NewOutputFormat: %w", err)
	}
	return format, nil
}

// NewJSONSchemaOutputFormat builds a named JSON Schema output-format contract
// and snapshots schema. Description can be assigned to the returned value.
func NewJSONSchemaOutputFormat(name string, schema json.RawMessage) (OutputFormat, error) {
	format := OutputFormat{Type: OutputFormatJSONSchema, Name: name, Schema: bytes.Clone(schema)}
	if err := format.Validate(); err != nil {
		return OutputFormat{}, fmt.Errorf("chat.NewJSONSchemaOutputFormat: %w", err)
	}
	return format, nil
}

// Clone returns an independent copy of o. A nil receiver returns nil.
func (o *OutputFormat) Clone() *OutputFormat {
	if o == nil {
		return nil
	}
	clone := *o
	clone.Schema = bytes.Clone(o.Schema)
	return &clone
}

// SchemaAs decodes o's JSON Schema into T. It lets protocol adapters choose
// the representation required by their SDK without taking ownership of schema
// validation or decoding. Only json_schema formats have a schema.
func (o *OutputFormat) SchemaAs[T any]() (T, error) {
	var zero T
	if o == nil {
		return zero, fmt.Errorf("%w: nil receiver", ErrInvalidOutputFormat)
	}
	if err := o.Validate(); err != nil {
		return zero, err
	}
	if o.Type != OutputFormatJSONSchema {
		return zero, fmt.Errorf("%w: %s output has no schema", ErrInvalidOutputFormat, o.Type)
	}
	var schema T
	if err := jsonv2.Unmarshal(o.Schema, &schema); err != nil {
		return zero, fmt.Errorf("chat.OutputFormat.SchemaAs: %w", err)
	}
	return schema, nil
}

// FallbackInstruction returns an equivalent model instruction for adapters
// whose native protocol cannot represent o. A nil or text format needs no
// instruction.
func (o *OutputFormat) FallbackInstruction() (string, error) {
	if o == nil || o.Type == OutputFormatText {
		return "", nil
	}
	if err := o.Validate(); err != nil {
		return "", err
	}
	const prefix = "Return only one valid JSON object. Do not include markdown fences, explanations, commentary, or leading or trailing text."
	if o.Type == OutputFormatJSON {
		return prefix, nil
	}
	var schema bytes.Buffer
	if err := json.Compact(&schema, o.Schema); err != nil {
		return "", fmt.Errorf("chat.OutputFormat.FallbackInstruction: compact schema: %w", err)
	}
	return prefix + " The object must conform to this JSON Schema:\n" + schema.String(), nil
}

// Validate verifies the output-format contract and its type-specific invariants.
func (o OutputFormat) Validate() error {
	switch o.Type {
	case OutputFormatText, OutputFormatJSON:
		if o.Name != "" || o.Description != "" || len(o.Schema) != 0 {
			return fmt.Errorf("%w: %s output must not define schema fields", ErrInvalidOutputFormat, o.Type)
		}
		return nil
	case OutputFormatJSONSchema:
		if o.Name == "" {
			return fmt.Errorf("%w: json_schema name must not be empty", ErrInvalidOutputFormat)
		}
		if strings.TrimSpace(o.Name) != o.Name {
			return fmt.Errorf("%w: json_schema name must not have surrounding whitespace", ErrInvalidOutputFormat)
		}
		if o.Description != "" && strings.TrimSpace(o.Description) != o.Description {
			return fmt.Errorf("%w: json_schema description must not have surrounding whitespace", ErrInvalidOutputFormat)
		}
		schema, err := corejsonschema.Parse(o.Schema)
		if err != nil {
			return fmt.Errorf("%w: json_schema schema: %w", ErrInvalidOutputFormat, err)
		}
		var object map[string]json.RawMessage
		if jsonv2.Unmarshal(schema.JSON(), &object) != nil || object == nil {
			return fmt.Errorf("%w: json_schema schema must be an object", ErrInvalidOutputFormat)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported type %q", ErrInvalidOutputFormat, o.Type)
	}
}

// MarshalJSON validates OutputFormat before writing its wire representation.
func (o OutputFormat) MarshalJSON() ([]byte, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	type wireOutputFormat OutputFormat
	return json.Marshal(wireOutputFormat(o))
}

// UnmarshalJSON decodes and validates OutputFormat before replacing the receiver.
func (o *OutputFormat) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidOutputFormat)
	}
	type wireOutputFormat OutputFormat
	var decoded wireOutputFormat
	if err := jsonv2.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidOutputFormat, err)
	}
	candidate := OutputFormat(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*o = candidate
	return nil
}
