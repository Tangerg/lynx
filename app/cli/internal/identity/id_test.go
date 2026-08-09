package identity

import (
	"strings"
	"testing"
)

func TestNewReturnsPrefixedDistinctIdentities(t *testing.T) {
	first, err := New("req")
	if err != nil {
		t.Fatal(err)
	}
	second, err := New("req")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first, "req_") || len(first) != 36 || first == second {
		t.Fatalf("identities = %q, %q", first, second)
	}
}
