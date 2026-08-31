package mistral

import (
	"encoding/json"
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestNewResponseFormat(t *testing.T) {
	format, err := corechat.NewJSONSchemaOutputFormat(corechat.JSONSchemaConfig{
		Name: "answer", Schema: json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := newResponseFormat(&format)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Type != outputFormatTypeJSONSchema {
		t.Fatalf("response format = %#v", mapped)
	}
	if mapped.JSONSchema == nil || mapped.JSONSchema.Name != "answer" || !mapped.JSONSchema.Strict {
		t.Fatalf("json_schema = %#v", mapped.JSONSchema)
	}
}
