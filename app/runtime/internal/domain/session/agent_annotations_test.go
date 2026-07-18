package session

import (
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
