package mcpserver

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestInputSchemaOwnsCanonicalJSONObject(t *testing.T) {
	source := []byte(`{"type":"object","properties":{"limit":{"maximum":9007199254740993,"type":"integer"}},"required":["limit"]}`)
	schema, err := ParseInputSchema(source)
	if err != nil {
		t.Fatalf("ParseInputSchema: %v", err)
	}
	source[2] = 'x'

	const want = `{"properties":{"limit":{"maximum":9007199254740993,"type":"integer"}},"required":["limit"],"type":"object"}`
	if got := schema.String(); got != want {
		t.Fatalf("String = %s, want %s", got, want)
	}

	projected := schema.Map()
	projected["type"] = "array"
	properties := projected["properties"].(map[string]any)
	limit := properties["limit"].(map[string]any)
	if got := limit["maximum"]; got != json.Number("9007199254740993") {
		t.Fatalf("maximum = %#v, want exact json.Number", got)
	}
	if got := schema.String(); got != want {
		t.Fatalf("Map exposed mutable storage: %s", got)
	}
}

func TestInputSchemaZeroValueIsUnconstrainedObject(t *testing.T) {
	var schema InputSchema
	if got := schema.String(); got != `{"type":"object"}` {
		t.Fatalf("String = %s, want object schema", got)
	}
	if got := schema.Map()["type"]; got != "object" {
		t.Fatalf("Map type = %#v, want object", got)
	}
}

func TestInputSchemaRejectsInvalidSchemas(t *testing.T) {
	for _, input := range []string{"", "null", "[]", `"value"`, "42", "{", `{}`, `{"type":"array"}`, `{"type":42}`} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseInputSchema([]byte(input)); !errors.Is(err, ErrInvalidInputSchema) {
				t.Fatalf("ParseInputSchema(%q) error = %v, want ErrInvalidInputSchema", input, err)
			}
		})
	}
}

func TestNewInputSchemaRejectsUnencodableValue(t *testing.T) {
	if _, err := NewInputSchema(nil); !errors.Is(err, ErrInvalidInputSchema) {
		t.Fatalf("NewInputSchema(nil) error = %v, want ErrInvalidInputSchema", err)
	}
	if _, err := NewInputSchema(make(chan int)); !errors.Is(err, ErrInvalidInputSchema) {
		t.Fatalf("NewInputSchema error = %v, want ErrInvalidInputSchema", err)
	}
}
