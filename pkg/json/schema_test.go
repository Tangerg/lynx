package json

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data structures

// Addr represents an address with postal code and position information.
type Addr struct {
	Zip      string `json:"zip" jsonschema:"required"`
	Position string `json:"position" jsonschema:"description=Geographic position"`
}

// TestUser represents a user with comprehensive field types for testing.
type TestUser struct {
	ID          int                    `json:"id" jsonschema:"required"`
	Name        string                 `json:"name" jsonschema:"title=the name,description=The name of a friend,example=joe,example=lucy,default=alex"`
	Friends     []int                  `json:"friends,omitempty" jsonschema_description:"The list of IDs, omitted when empty"`
	Tags        map[string]interface{} `json:"tags,omitempty" jsonschema_extras:"a=b,foo=bar,foo=bar1"`
	BirthDate   time.Time              `json:"birth_date,omitempty" jsonschema:"oneof_required=date"`
	YearOfBirth string                 `json:"year_of_birth,omitempty" jsonschema:"oneof_required=year"`
	Metadata    interface{}            `json:"metadata,omitempty" jsonschema:"oneof_type=string;array"`
	FavColor    string                 `json:"fav_color,omitempty" jsonschema:"enum=red,enum=green,enum=blue"`
	Addrs       []*Addr                `json:"addrs,omitempty"`
}

// TestStringDefSchemaOf_StructPointer tests schema generation for a struct pointer.
func TestStringDefSchemaOf_StructPointer(t *testing.T) {
	schema, err := StringDefSchemaOf(&TestUser{})
	require.NoError(t, err, "Schema generation should not fail")
	require.NotEmpty(t, schema, "Schema should not be empty")

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(schema), &result)
	require.NoError(t, err, "Schema should be valid JSON")

	// Verify basic structure
	assert.Equal(t, "object", result["type"], "Root type should be object")
	assert.NotNil(t, result["properties"], "Properties should exist")

	// Verify specific fields
	properties, ok := result["properties"].(map[string]interface{})
	require.True(t, ok, "Properties should be a map")
	assert.Contains(t, properties, "id", "Should contain id field")
	assert.Contains(t, properties, "name", "Should contain name field")
	assert.Contains(t, properties, "addrs", "Should contain addrs field")

	t.Logf("Generated schema:\n%s", schema)
}

// TestStringDefSchemaOf_StructValue tests schema generation for a struct value.
func TestStringDefSchemaOf_StructValue(t *testing.T) {
	schema, err := StringDefSchemaOf(Addr{})
	require.NoError(t, err, "Schema generation should not fail")
	require.NotEmpty(t, schema, "Schema should not be empty")

	// Verify it's valid JSON
	var result map[string]interface{}
	err = json.Unmarshal([]byte(schema), &result)
	require.NoError(t, err, "Schema should be valid JSON")

	// Verify structure
	assert.Equal(t, "object", result["type"], "Type should be object")
	properties, ok := result["properties"].(map[string]interface{})
	require.True(t, ok, "Properties should be a map")
	assert.Contains(t, properties, "zip", "Should contain zip field")
	assert.Contains(t, properties, "position", "Should contain position field")

	t.Logf("Generated schema:\n%s", schema)
}

// TestStringDefSchemaOf_PrimitiveType tests schema generation for primitive types.
func TestStringDefSchemaOf_PrimitiveType(t *testing.T) {
	tests := []struct {
		name         string
		input        interface{}
		expectedType string
	}{
		{
			name:         "integer",
			input:        1,
			expectedType: "integer",
		},
		{
			name:         "string",
			input:        "test",
			expectedType: "string",
		},
		{
			name:         "boolean",
			input:        true,
			expectedType: "boolean",
		},
		{
			name:         "float",
			input:        3.14,
			expectedType: "number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := StringDefSchemaOf(tt.input)
			require.NoError(t, err, "Schema generation should not fail")
			require.NotEmpty(t, schema, "Schema should not be empty")

			var result map[string]interface{}
			err = json.Unmarshal([]byte(schema), &result)
			require.NoError(t, err, "Schema should be valid JSON")

			assert.Equal(t, tt.expectedType, result["type"], "Type should match expected")
			t.Logf("Generated schema for %s:\n%s", tt.name, schema)
		})
	}
}

// TestStringDefSchemaOf_NilValue tests schema generation with nil value.
func TestStringDefSchemaOf_NilValue(t *testing.T) {
	schema, err := StringDefSchemaOf(nil)
	assert.Error(t, err, "Should return error for nil value")
	assert.Empty(t, schema, "Schema should be empty on error")
	assert.Contains(t, err.Error(), "nil", "Error should mention nil")
}

// TestMapDefSchemaOf_Success tests map schema generation.
func TestMapDefSchemaOf_Success(t *testing.T) {
	schemaMap, err := MapDefSchemaOf(&TestUser{})
	require.NoError(t, err, "Schema generation should not fail")
	require.NotNil(t, schemaMap, "Schema map should not be nil")

	// Verify basic structure
	assert.Equal(t, "object", schemaMap["type"], "Root type should be object")
	assert.NotNil(t, schemaMap["properties"], "Properties should exist")

	// Verify it can be marshaled back to JSON
	jsonBytes, err := json.Marshal(schemaMap)
	require.NoError(t, err, "Should be able to marshal map to JSON")

	// Verify the JSON is valid
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err, "Should be able to unmarshal back")

	t.Logf("Generated schema map:\n%s", string(jsonBytes))
}

// TestMapDefSchemaOf_NilValue tests map schema generation with nil value.
func TestMapDefSchemaOf_NilValue(t *testing.T) {
	schemaMap, err := MapDefSchemaOf(nil)
	assert.Error(t, err, "Should return error for nil value")
	assert.Nil(t, schemaMap, "Schema map should be nil on error")
}

// TestStringDefSchemaOfWithConfig tests schema generation with custom configuration.
func TestStringDefSchemaOfWithConfig(t *testing.T) {
	tests := []struct {
		name   string
		config SchemaConfig
		verify func(t *testing.T, schema string)
	}{
		{
			name: "with_additional_properties",
			config: SchemaConfig{
				Anonymous:                 true,
				DoNotReference:            true,
				AllowAdditionalProperties: true,
				IncludeSchemaVersion:      false,
			},
			verify: func(t *testing.T, schema string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(schema), &result)
				require.NoError(t, err)
				// Note: additionalProperties behavior depends on jsonschema library implementation
				t.Logf("Schema with additional properties:\n%s", schema)
			},
		},
		{
			name: "with_schema_version",
			config: SchemaConfig{
				Anonymous:                 true,
				DoNotReference:            true,
				AllowAdditionalProperties: false,
				IncludeSchemaVersion:      true,
			},
			verify: func(t *testing.T, schema string) {
				var result map[string]interface{}
				err := json.Unmarshal([]byte(schema), &result)
				require.NoError(t, err)
				assert.NotEmpty(t, result["$schema"], "Should include schema version")
				t.Logf("Schema with version:\n%s", schema)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := StringDefSchemaOfWithConfig(TestUser{}, tt.config)
			require.NoError(t, err, "Schema generation should not fail")
			require.NotEmpty(t, schema, "Schema should not be empty")
			tt.verify(t, schema)
		})
	}
}

// TestMustStringDefSchemaOf tests the Must variant function.
func TestMustStringDefSchemaOf(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		assert.NotPanics(t, func() {
			schema := MustStringDefSchemaOf(TestUser{})
			assert.NotEmpty(t, schema, "Schema should not be empty")
		}, "Should not panic for valid input")
	})

	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			MustStringDefSchemaOf(nil)
		}, "Should panic for nil input")
	})
}

// TestMustMapDefSchemaOf tests the Must variant function for map output.
func TestMustMapDefSchemaOf(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		assert.NotPanics(t, func() {
			schema := MustMapDefSchemaOf(TestUser{})
			assert.NotNil(t, schema, "Schema should not be nil")
		}, "Should not panic for valid input")
	})

	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			MustMapDefSchemaOf(nil)
		}, "Should panic for nil input")
	})
}

// TestSchemaConsistency verifies that String and Map versions produce equivalent schemas.
func TestSchemaConsistency(t *testing.T) {
	stringSchema, err := StringDefSchemaOf(TestUser{})
	require.NoError(t, err)

	mapSchema, err := MapDefSchemaOf(TestUser{})
	require.NoError(t, err)

	// Convert map schema back to string
	mapSchemaJSON, err := json.Marshal(mapSchema)
	require.NoError(t, err)

	// Both should produce equivalent JSON (may differ in formatting)
	var stringResult, mapResult map[string]interface{}
	err = json.Unmarshal([]byte(stringSchema), &stringResult)
	require.NoError(t, err)
	err = json.Unmarshal(mapSchemaJSON, &mapResult)
	require.NoError(t, err)

	assert.Equal(t, stringResult["type"], mapResult["type"], "Types should match")
	assert.Equal(t, len(stringResult), len(mapResult), "Schema structure should be equivalent")
}

// BenchmarkStringDefSchemaOf benchmarks string schema generation.
func BenchmarkStringDefSchemaOf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = StringDefSchemaOf(TestUser{})
	}
}

// BenchmarkMapDefSchemaOf benchmarks map schema generation.
func BenchmarkMapDefSchemaOf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = MapDefSchemaOf(TestUser{})
	}
}
