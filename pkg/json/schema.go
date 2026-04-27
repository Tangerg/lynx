package json

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// SchemaConfig controls JSON Schema generation. The zero value is
// usable; [DefaultSchemaConfig] returns the recommended defaults.
type SchemaConfig struct {
	// Anonymous emits anonymous schemas (no $id).
	Anonymous bool
	// ExpandedStruct inlines struct definitions instead of referencing.
	ExpandedStruct bool
	// DoNotReference disables $ref usage; all definitions are inlined.
	DoNotReference bool
	// AllowAdditionalProperties allows fields not declared in the schema.
	AllowAdditionalProperties bool
	// IncludeSchemaVersion keeps the $schema field in the output.
	IncludeSchemaVersion bool
}

// DefaultSchemaConfig returns the configuration used by
// [StringDefSchemaOf] and [MapDefSchemaOf]: anonymous, fully inlined,
// strict (no extra fields), no $schema header.
func DefaultSchemaConfig() SchemaConfig {
	return SchemaConfig{
		Anonymous:      true,
		DoNotReference: true,
	}
}

// StringDefSchemaOf returns a JSON Schema for v as a JSON string,
// using [DefaultSchemaConfig].
//
// Example:
//
//	type User struct {
//	    Name string `json:"name" jsonschema:"required"`
//	    Age  int    `json:"age"`
//	}
//	schema, _ := json.StringDefSchemaOf(User{})
func StringDefSchemaOf(v any) (string, error) {
	return StringDefSchemaOfWithConfig(v, DefaultSchemaConfig())
}

// StringDefSchemaOfWithConfig is like [StringDefSchemaOf] but uses cfg.
func StringDefSchemaOfWithConfig(v any, cfg SchemaConfig) (string, error) {
	schema, err := generateSchema(v, cfg)
	if err != nil {
		return "", fmt.Errorf("generate schema: %w", err)
	}
	raw, err := schema.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshal schema: %w", err)
	}
	return string(raw), nil
}

// MapDefSchemaOf returns a JSON Schema for v decoded into a generic
// map, using [DefaultSchemaConfig].
func MapDefSchemaOf(v any) (map[string]any, error) {
	return MapDefSchemaOfWithConfig(v, DefaultSchemaConfig())
}

// MapDefSchemaOfWithConfig is like [MapDefSchemaOf] but uses cfg.
func MapDefSchemaOfWithConfig(v any, cfg SchemaConfig) (map[string]any, error) {
	schema, err := generateSchema(v, cfg)
	if err != nil {
		return nil, fmt.Errorf("generate schema: %w", err)
	}
	raw, err := schema.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return m, nil
}

// MustStringDefSchemaOf is the panicking variant of [StringDefSchemaOf].
func MustStringDefSchemaOf(v any) string {
	out, err := StringDefSchemaOf(v)
	if err != nil {
		panic(fmt.Sprintf("json: schema generation failed: %v", err))
	}
	return out
}

// MustMapDefSchemaOf is the panicking variant of [MapDefSchemaOf].
func MustMapDefSchemaOf(v any) map[string]any {
	out, err := MapDefSchemaOf(v)
	if err != nil {
		panic(fmt.Sprintf("json: schema generation failed: %v", err))
	}
	return out
}

// generateSchema reflects a *jsonschema.Schema for v with the given
// configuration. Struct values are always emitted with ExpandedStruct
// set so the result is a single, self-contained object.
func generateSchema(v any, cfg SchemaConfig) (*jsonschema.Schema, error) {
	if v == nil {
		return nil, fmt.Errorf("nil value")
	}
	r := &jsonschema.Reflector{
		Anonymous:                 cfg.Anonymous,
		ExpandedStruct:            cfg.ExpandedStruct,
		DoNotReference:            cfg.DoNotReference,
		AllowAdditionalProperties: cfg.AllowAdditionalProperties,
	}
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		r.ExpandedStruct = true
	}
	schema := r.Reflect(v)
	if schema == nil {
		return nil, fmt.Errorf("reflect schema for %T", v)
	}
	if !cfg.IncludeSchemaVersion {
		schema.Version = ""
	}
	return schema, nil
}
