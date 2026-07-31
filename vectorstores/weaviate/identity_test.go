package weaviate

import (
	"errors"
	"testing"
)

func TestValidateObjectID(t *testing.T) {
	if err := validateObjectID("12345678-1234-1234-1234-123456789012"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "document-one", "42"} {
		if err := validateObjectID(id); !errors.Is(err, ErrInvalidObjectID) {
			t.Fatalf("validateObjectID(%q) error = %v, want ErrInvalidObjectID", id, err)
		}
	}
}
