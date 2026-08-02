package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/agent/internal/schema"
)

func TestString(t *testing.T) {
	type input struct {
		Name string `json:"name" jsonschema:"required"`
	}

	encoded, err := schema.String(input{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["type"] != "object" || decoded["$schema"] != nil || decoded["additionalProperties"] != false {
		t.Fatalf("schema = %s, want strict object without version", encoded)
	}
	if _, err := schema.String(nil); err == nil {
		t.Fatal("nil value generated a schema")
	}
}
