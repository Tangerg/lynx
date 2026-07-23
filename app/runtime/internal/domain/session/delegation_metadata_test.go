package session

import (
	"errors"
	"testing"
)

func TestDelegationMetadataOwnsCanonicalJSONObject(t *testing.T) {
	source := []byte(`{"z":[1,true],"a":{"name":"agent"}}`)
	metadata, err := ParseDelegationMetadata(source)
	if err != nil {
		t.Fatalf("ParseDelegationMetadata: %v", err)
	}
	source[2] = 'x'

	const want = `{"a":{"name":"agent"},"z":[1,true]}`
	if got := metadata.String(); got != want {
		t.Fatalf("String = %s, want %s", got, want)
	}
	encoded := metadata.JSON()
	encoded[2] = 'x'
	if got := metadata.String(); got != want {
		t.Fatalf("JSON exposed mutable storage: %s", got)
	}
}

func TestDelegationMetadataZeroValueIsEmptyObject(t *testing.T) {
	var metadata DelegationMetadata
	if got := metadata.String(); got != "{}" {
		t.Fatalf("String = %s, want {}", got)
	}
	if got := string(metadata.JSON()); got != "{}" {
		t.Fatalf("JSON = %s, want {}", got)
	}
}

func TestDelegationMetadataRejectsNonObjects(t *testing.T) {
	for _, input := range []string{"", "null", "[]", `"value"`, "42", "{"} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseDelegationMetadata([]byte(input)); !errors.Is(err, ErrInvalidDelegationMetadata) {
				t.Fatalf("ParseDelegationMetadata(%q) error = %v, want ErrInvalidDelegationMetadata", input, err)
			}
		})
	}
}
