package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

type jsonWireFixture struct {
	Metadata  map[string]json.RawMessage `json:"metadata"`
	Signature []byte                     `json:"signature"`
}

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

func TestSchemaForMatchesEncodingJSONWireTypes(t *testing.T) {
	schema, err := SchemaFor[jsonWireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	valid, err := EncodeOutput(jsonWireFixture{
		Metadata: map[string]json.RawMessage{
			"array":   json.RawMessage(`[1,"two"]`),
			"boolean": json.RawMessage(`true`),
			"null":    json.RawMessage(`null`),
			"number":  json.RawMessage(`3`),
			"object":  json.RawMessage(`{"id":"chunk"}`),
			"string":  json.RawMessage(`"deepseek"`),
		},
		Signature: []byte{0, 1, 2, 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateOutput(valid); err != nil {
		t.Fatalf("ValidateOutput(valid JSON wire values) error = %v; schema = %s", err, schema.JSON())
	}

	nilSignature, err := EncodeOutput(jsonWireFixture{Metadata: map[string]json.RawMessage{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateOutput(nilSignature); err != nil {
		t.Fatalf("ValidateOutput(nil byte slice) error = %v; schema = %s", err, schema.JSON())
	}

	arraySignature, err := ParseOutput([]byte(`{"metadata":{"provider":"deepseek"},"signature":[1,2,3]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateOutput(arraySignature); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("ValidateOutput(array signature) error = %v, want ErrInvalidOutput", err)
	}
	if !bytes.Contains(schema.JSON(), []byte(`"contentEncoding":"base64"`)) {
		t.Fatalf("derived schema does not identify the byte-slice encoding: %s", schema.JSON())
	}

	_, err = EncodeOutput(jsonWireFixture{
		Metadata: map[string]json.RawMessage{"invalid": json.RawMessage(`{`)},
	})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("EncodeOutput(invalid RawMessage) error = %v, want ErrInvalidOutput", err)
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
