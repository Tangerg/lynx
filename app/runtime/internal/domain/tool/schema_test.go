package tool

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSchemaOwnsCanonicalObject(t *testing.T) {
	source := []byte(`{"type":"object","properties":{"limit":{"maximum":9007199254740993}},"required":["limit"]}`)
	schema, err := ParseSchema(source)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	source[2] = 'x'

	const want = `{"properties":{"limit":{"maximum":9007199254740993}},"required":["limit"],"type":"object"}`
	if got := schema.String(); got != want {
		t.Fatalf("String = %s, want %s", got, want)
	}
	projected := schema.Map()
	maximum := projected["properties"].(map[string]any)["limit"].(map[string]any)["maximum"]
	if maximum != json.Number("9007199254740993") {
		t.Fatalf("maximum = %#v, want exact json.Number", maximum)
	}
	projected["type"] = "array"
	if got := schema.String(); got != want {
		t.Fatalf("Map exposed mutable storage: %s", got)
	}
}

func TestSchemaZeroValueIsEmptyObject(t *testing.T) {
	var schema Schema
	if got := schema.String(); got != "{}" {
		t.Fatalf("String = %s, want {}", got)
	}
	if got := schema.Map(); got == nil || len(got) != 0 {
		t.Fatalf("Map = %#v, want non-nil empty object", got)
	}
}

func TestSchemaRejectsNonObjectsAndMalformedDocuments(t *testing.T) {
	for _, input := range []string{"", "null", "[]", `"schema"`, "42", "{", `{} {}`} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseSchema([]byte(input)); !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("ParseSchema(%q) error = %v, want ErrInvalidSchema", input, err)
			}
		})
	}
}
