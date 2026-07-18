package session

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestAgentAnnotationsOwnsCanonicalJSONObject(t *testing.T) {
	source := []byte(`{"z":[1,true],"a":{"name":"agent"}}`)
	annotations, err := ParseAgentAnnotations(source)
	if err != nil {
		t.Fatalf("ParseAgentAnnotations: %v", err)
	}
	source[2] = 'x'

	const want = `{"a":{"name":"agent"},"z":[1,true]}`
	if got := annotations.String(); got != want {
		t.Fatalf("String = %s, want %s", got, want)
	}
	encoded := annotations.JSON()
	encoded[2] = 'x'
	if got := annotations.String(); got != want {
		t.Fatalf("JSON exposed mutable storage: %s", got)
	}
}

func TestAgentAnnotationsZeroValueIsEmptyObject(t *testing.T) {
	var annotations AgentAnnotations
	if got := annotations.String(); got != "{}" {
		t.Fatalf("String = %s, want {}", got)
	}
	if got := string(annotations.JSON()); got != "{}" {
		t.Fatalf("JSON = %s, want {}", got)
	}
	if got := annotations.Map(); len(got) != 0 || got == nil {
		t.Fatalf("Map = %#v, want non-nil empty object", got)
	}
}

func TestAgentAnnotationsRejectsNonObjects(t *testing.T) {
	for _, input := range []string{"", "null", "[]", `"value"`, "42", "{"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseAgentAnnotations([]byte(input)); !errors.Is(err, ErrInvalidAgentAnnotations) {
				t.Fatalf("ParseAgentAnnotations(%q) error = %v, want ErrInvalidAgentAnnotations", input, err)
			}
		})
	}
}

func TestAgentAnnotationsMapRoundTripPreservesNumbersAndOwnership(t *testing.T) {
	source := map[string]any{
		"sequence": json.Number("9007199254740993"),
		"nested":   map[string]any{"enabled": true},
	}
	annotations, err := AgentAnnotationsFromMap(source)
	if err != nil {
		t.Fatalf("AgentAnnotationsFromMap: %v", err)
	}
	source["sequence"] = 1

	projected := annotations.Map()
	if got := projected["sequence"]; got != json.Number("9007199254740993") {
		t.Fatalf("sequence = %#v, want exact json.Number", got)
	}
	projected["nested"].(map[string]any)["enabled"] = false
	if got := annotations.Map()["nested"].(map[string]any)["enabled"]; got != true {
		t.Fatalf("Map exposed mutable storage: enabled = %#v", got)
	}
}

func TestAgentAnnotationsFromMapRejectsUnencodableValue(t *testing.T) {
	if _, err := AgentAnnotationsFromMap(map[string]any{"invalid": make(chan int)}); !errors.Is(err, ErrInvalidAgentAnnotations) {
		t.Fatalf("AgentAnnotationsFromMap error = %v, want ErrInvalidAgentAnnotations", err)
	}
}
