package tool_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/tool"
)

func TestSchemaDerivesStrictStructContract(t *testing.T) {
	type option struct {
		Label string `json:"label" jsonschema:"required"`
	}
	type input struct {
		Operation string    `json:"operation" jsonschema:"required,enum=list,enum=load" jsonschema_description:"Operation to run."`
		Options   []option  `json:"options,omitempty" jsonschema:"minItems=1,maxItems=4"`
		Ignored   string    `json:"-"`
		When      time.Time `json:"when,omitempty"`
	}

	encoded, err := tool.Schema[input]()
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(encoded), &schema); err != nil {
		t.Fatal(err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema = %s, want strict object", encoded)
	}
	properties := schema["properties"].(map[string]any)
	if _, ok := properties["Ignored"]; ok {
		t.Fatalf("schema includes ignored field: %s", encoded)
	}
	operation := properties["operation"].(map[string]any)
	if operation["description"] != "Operation to run." || len(operation["enum"].([]any)) != 2 {
		t.Fatalf("operation schema = %#v", operation)
	}
	options := properties["options"].(map[string]any)
	if options["minItems"] != float64(1) || options["maxItems"] != float64(4) {
		t.Fatalf("options schema = %#v", options)
	}
	when := properties["when"].(map[string]any)
	if when["type"] != "string" || when["format"] != "date-time" {
		t.Fatalf("time schema = %#v", when)
	}
}

func TestSchemaSupportsCollectionsAndPointers(t *testing.T) {
	type input struct {
		Names  []string          `json:"names"`
		Labels map[string]string `json:"labels"`
		Limit  *int              `json:"limit,omitempty"`
		Value  any               `json:"value,omitempty"`
	}
	if _, err := tool.Schema[*input](); err != nil {
		t.Fatal(err)
	}
	if schema, err := tool.Schema[map[string][]int](); err != nil || !strings.Contains(schema, `"additionalProperties":{"type":"array"`) {
		t.Fatalf("map schema = %q, %v", schema, err)
	}
}

func TestSchemaRejectsUnsupportedContracts(t *testing.T) {
	type recursive struct {
		Next *recursive `json:"next,omitempty"`
	}
	type badTag struct {
		Value string `json:"value" jsonschema:"minimum=1"`
	}
	if _, err := tool.Schema[recursive](); err == nil {
		t.Fatal("recursive schema succeeded")
	}
	if _, err := tool.Schema[badTag](); err == nil {
		t.Fatal("unsupported schema tag succeeded")
	}
	if _, err := tool.Schema[chan int](); err == nil {
		t.Fatal("unsupported Go type succeeded")
	}
}
