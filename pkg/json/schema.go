package json

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// SchemaConfig holds the configuration for JSON schema generation.
type SchemaConfig struct {
	// Anonymous generates anonymous schemas without using references
	Anonymous bool
	// ExpandedStruct expands struct definitions inline instead of referencing
	ExpandedStruct bool
	// DoNotReference disables $ref usage and inlines all definitions
	DoNotReference bool
	// AllowAdditionalProperties allows properties not defined in the schema
	AllowAdditionalProperties bool
	// IncludeSchemaVersion includes the $schema version field in output
	IncludeSchemaVersion bool
}

// DefaultSchemaConfig returns the default configuration for schema generation.
func DefaultSchemaConfig() SchemaConfig {
	return SchemaConfig{
		Anonymous:                 true,
		ExpandedStruct:            false,
		DoNotReference:            true,
		AllowAdditionalProperties: false,
		IncludeSchemaVersion:      false,
	}
}

// StringDefSchemaOf generates a JSON schema definition string for a given value.
// It returns the schema as a JSON string and an error if generation fails.
//
// Parameters:
//   - v: the value to generate schema for (typically a struct or basic type)
//
// Returns:
//   - string: JSON schema definition as a string
//   - error: error if schema generation or marshaling fails
//
// Example:
//
//	type User struct {
//	    Name string `json:"name" jsonschema:"required"`
//	    Age  int    `json:"age"`
//	}
//	schema, err := StringDefSchemaOf(User{})
func StringDefSchemaOf(v any) (string, error) {
	return StringDefSchemaOfWithConfig(v, DefaultSchemaConfig())
}

// StringDefSchemaOfWithConfig generates a JSON schema string with custom configuration.
func StringDefSchemaOfWithConfig(v any, config SchemaConfig) (string, error) {
	schema, err := generateSchema(v, config)
	if err != nil {
		return "", fmt.Errorf("generate schema: %w", err)
	}

	marshalJSON, err := schema.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("marshal schema to JSON: %w", err)
	}

	return string(marshalJSON), nil
}

// MapDefSchemaOf generates a JSON schema definition as a map for a given value.
// It returns the schema as a map[string]any and an error if generation fails.
//
// Parameters:
//   - v: the value to generate schema for (typically a struct or basic type)
//
// Returns:
//   - map[string]any: JSON schema definition as a map
//   - error: error if schema generation, marshaling, or unmarshaling fails
//
// Example:
//
//	type User struct {
//	    Name string `json:"name"`
//	}
//	schemaMap, err := MapDefSchemaOf(User{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	properties := schemaMap["properties"]
func MapDefSchemaOf(v any) (map[string]any, error) {
	return MapDefSchemaOfWithConfig(v, DefaultSchemaConfig())
}

// MapDefSchemaOfWithConfig generates a JSON schema map with custom configuration.
func MapDefSchemaOfWithConfig(v any, config SchemaConfig) (map[string]any, error) {
	schema, err := generateSchema(v, config)
	if err != nil {
		return nil, fmt.Errorf("generate schema: %w", err)
	}

	raw, err := schema.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal schema to JSON: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal schema to map: %w", err)
	}

	return m, nil
}

// generateSchema is a helper function that creates a JSON schema from a given value
// using the provided configuration. It handles type reflection and applies
// appropriate settings based on the input type.
//
// Parameters:
//   - v: the value to generate schema for
//   - config: configuration options for schema generation
//
// Returns:
//   - *jsonschema.Schema: the generated schema object
//   - error: error if schema generation fails
func generateSchema(v any, config SchemaConfig) (*jsonschema.Schema, error) {
	// Validate input
	if v == nil {
		return nil, fmt.Errorf("cannot generate schema for nil value")
	}

	// Create reflector with configuration
	r := &jsonschema.Reflector{
		Anonymous:                 config.Anonymous,
		ExpandedStruct:            config.ExpandedStruct,
		DoNotReference:            config.DoNotReference,
		AllowAdditionalProperties: config.AllowAdditionalProperties,
	}

	// Check if the value is a struct type and expand if necessary
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() == reflect.Struct {
		r.ExpandedStruct = true
	}

	// Generate schema using reflection
	schema := r.Reflect(v)
	if schema == nil {
		return nil, fmt.Errorf("failed to reflect schema for type %T", v)
	}

	// Remove schema version if not required
	if !config.IncludeSchemaVersion {
		schema.Version = ""
	}

	return schema, nil
}

// MustStringDefSchemaOf is a convenience wrapper around StringDefSchemaOf that panics on error.
// Use this only when you are certain the schema generation will succeed.
//
// Example:
//
//	type Config struct {
//	    Port int `json:"port"`
//	}
//	schema := MustStringDefSchemaOf(Config{})
func MustStringDefSchemaOf(v any) string {
	schema, err := StringDefSchemaOf(v)
	if err != nil {
		panic(fmt.Sprintf("failed to generate schema: %v", err))
	}
	return schema
}

// MustMapDefSchemaOf is a convenience wrapper around MapDefSchemaOf that panics on error.
// Use this only when you are certain the schema generation will succeed.
//
// Example:
//
//	type Config struct {
//	    Port int `json:"port"`
//	}
//	schemaMap := MustMapDefSchemaOf(Config{})
func MustMapDefSchemaOf(v any) map[string]any {
	schema, err := MapDefSchemaOf(v)
	if err != nil {
		panic(fmt.Sprintf("failed to generate schema: %v", err))
	}
	return schema
}
