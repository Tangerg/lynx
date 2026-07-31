package qdrant

import (
	"errors"
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"
)

func TestPointIDRoundTrip(t *testing.T) {
	for _, id := range []string{"0", "42", "12345678-1234-1234-1234-123456789012"} {
		pointID, err := parsePointID(id)
		if err != nil {
			t.Fatalf("parsePointID(%q): %v", id, err)
		}
		got, err := formatPointID(pointID)
		if err != nil {
			t.Fatalf("formatPointID(parsePointID(%q)): %v", id, err)
		}
		if got != id {
			t.Fatalf("formatPointID(parsePointID(%q)) = %q", id, got)
		}
	}
}

func TestFormatPointIDRejectsMissingVariant(t *testing.T) {
	for _, id := range []*qdrantclient.PointId{nil, {}} {
		if _, err := formatPointID(id); !errors.Is(err, ErrInvalidPointID) {
			t.Fatalf("formatPointID(%v) error = %v, want ErrInvalidPointID", id, err)
		}
	}
}

func TestPointIDRejectsNonCanonicalValues(t *testing.T) {
	for _, id := range []string{"", "01", "document-one", "18446744073709551616"} {
		if _, err := parsePointID(id); !errors.Is(err, ErrInvalidPointID) {
			t.Fatalf("parsePointID(%q) error = %v, want ErrInvalidPointID", id, err)
		}
	}
}
