package diagnostictool

import (
	"encoding/json"
	"testing"
)

func TestDescriptorRejectsUnsafeAndMalformedTools(t *testing.T) {
	valid := Descriptor{Name: "inspect", Safety: Safe, Schema: json.RawMessage(`{"type":"object"}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid descriptor: %v", err)
	}
	for name, descriptor := range map[string]Descriptor{
		"empty name": {Safety: Safe, Schema: json.RawMessage(`{}`)},
		"unsafe":     {Name: "write", Safety: "write", Schema: json.RawMessage(`{}`)},
		"array":      {Name: "inspect", Safety: Safe, Schema: json.RawMessage(`[]`)},
		"malformed":  {Name: "inspect", Safety: Safe, Schema: json.RawMessage(`{`)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := descriptor.Validate(); err == nil {
				t.Fatal("Validate accepted malformed descriptor")
			}
		})
	}
}

func TestInvocationRequiresConfinedJSONObject(t *testing.T) {
	valid := Invocation{Tool: Descriptor{Name: "inspect", Safety: Safe, Schema: json.RawMessage(`{}`)}, Workspace: "/repo", Arguments: json.RawMessage(`{}`)}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid invocation: %v", err)
	}
	valid.Arguments = json.RawMessage(`null`)
	if err := valid.Validate(); err == nil {
		t.Fatal("Validate accepted non-object arguments")
	}
}

func TestParseArgumentsDefaultsAndRejectsNonObjects(t *testing.T) {
	arguments, err := ParseArguments("")
	if err != nil || string(arguments) != `{}` {
		t.Fatalf("ParseArguments empty = (%s, %v)", arguments, err)
	}
	if _, err := ParseArguments(`[]`); err == nil {
		t.Fatal("ParseArguments accepted an array")
	}
}
