package agent2

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSchemaForValidatesTypedWireValues(t *testing.T) {
	schema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := EncodeInput(wireFixture{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateInput(valid); err != nil {
		t.Fatalf("ValidateInput(valid) error = %v", err)
	}
	invalid, err := ParseInput([]byte(`{"message":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateInput(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidateInput(invalid) error = %v, want ErrInvalidInput; schema = %s", err, schema.JSON())
	}
}

func TestSchemaOwnsWireBytes(t *testing.T) {
	raw := json.RawMessage(`{"type":"string"}`)
	schema, err := ParseSchema(raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[2] = 'x'
	if got := string(schema.JSON()); got != `{"type":"string"}` {
		t.Fatalf("Schema.JSON() = %s", got)
	}
}

func TestSchemaRejectsInvalidDefinitions(t *testing.T) {
	for _, data := range []json.RawMessage{
		nil,
		[]byte(`[]`),
		[]byte(`{"type":"not-a-type"}`),
		[]byte(`{"minLength":-1}`),
		[]byte(`{"$ref":"https://example.com/external-schema"}`),
	} {
		if _, err := ParseSchema(data); !errors.Is(err, ErrInvalidSchema) {
			t.Fatalf("ParseSchema(%q) error = %v, want ErrInvalidSchema", data, err)
		}
	}
}
