package tool_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/tool"
)

func TestSchemaDerivesStrictStructContract(t *testing.T) {
	type option struct {
		Label string `json:"label"`
	}
	type input struct {
		Operation string    `json:"operation" jsonschema:"enum=list,enum=load,minLength=4,maxLength=4,pattern=^[a-z]+$" jsonschema_description:"Operation to run."`
		Options   []option  `json:"options,omitempty" jsonschema:"minItems=1,maxItems=4"`
		Ignored   string    `json:"-"`
		When      time.Time `json:"when,omitempty"`
		Score     float64   `json:"score,omitempty" jsonschema:"minimum=-1.5,maximum=2.5"`
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
	if operation["description"] != "Operation to run." || len(operation["enum"].([]any)) != 2 ||
		operation["minLength"] != float64(4) || operation["maxLength"] != float64(4) || operation["pattern"] != "^[a-z]+$" {
		t.Fatalf("operation schema = %#v", operation)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "operation" {
		t.Fatalf("required = %#v, want [operation]", required)
	}
	options := properties["options"].(map[string]any)
	if options["minItems"] != float64(1) || options["maxItems"] != float64(4) {
		t.Fatalf("options schema = %#v", options)
	}
	when := properties["when"].(map[string]any)
	if when["type"] != "string" || when["format"] != "date-time" {
		t.Fatalf("time schema = %#v", when)
	}
	score := properties["score"].(map[string]any)
	if score["minimum"] != -1.5 || score["maximum"] != 2.5 {
		t.Fatalf("score schema = %#v", score)
	}
}

func TestSchemaSupportsCollectionsAndPointers(t *testing.T) {
	type input struct {
		Names  []string          `json:"names"`
		Labels map[string]string `json:"labels"`
		Limit  *int              `json:"limit,omitempty"`
		Value  any               `json:"value,omitempty"`
		Debug  bool              `json:"debug,omitzero"`
	}
	if _, err := tool.Schema[*input](); err != nil {
		t.Fatal(err)
	}
	encoded, err := tool.Schema[map[string][]int]()
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(encoded, &schema); err != nil {
		t.Fatal(err)
	}
	additional := schema["additionalProperties"].(map[string]any)
	if additional["type"] != "array" {
		t.Fatalf("map schema = %s", encoded)
	}
}

func TestSchemaReturnsIndependentJSON(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	first, err := tool.Schema[input]()
	if err != nil {
		t.Fatal(err)
	}
	second, err := tool.Schema[input]()
	if err != nil {
		t.Fatal(err)
	}
	first[0] = '['
	if second[0] != '{' {
		t.Fatal("mutating returned schema changed another schema snapshot")
	}
}

func TestSchemaUsesUpstreamJSONSchemaSemantics(t *testing.T) {
	type recursive struct {
		Next *recursive `json:"next,omitempty"`
	}
	type badPattern struct {
		Value string `json:"value" jsonschema:"pattern=["`
	}
	type stringEncoded struct {
		Value int `json:"value,string"`
	}
	recursiveSchema, err := tool.Schema[recursive]()
	if err != nil {
		t.Fatalf("recursive schema: %v", err)
	}
	if !strings.Contains(string(recursiveSchema), `"$ref"`) || !strings.Contains(string(recursiveSchema), `"$defs"`) {
		t.Fatalf("recursive schema does not use references: %s", recursiveSchema)
	}
	if _, schemaErr := tool.Schema[badPattern](); schemaErr == nil {
		t.Fatal("invalid pattern succeeded")
	}
	encoded, err := tool.Schema[stringEncoded]()
	if err != nil {
		t.Fatalf("encoding/json string option: %v", err)
	}
	if !strings.Contains(string(encoded), `"value":{"type":"string"}`) {
		t.Fatalf("string-encoded field schema = %s", encoded)
	}
	if _, err := tool.Schema[chan int](); err == nil {
		t.Fatal("unsupported Go type succeeded")
	}
}
