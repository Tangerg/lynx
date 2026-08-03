package options

import (
	"strings"
	"testing"
)

func TestRejectUnsupported(t *testing.T) {
	if err := RejectUnsupported("provider", map[string]bool{"ignored": false}); err != nil {
		t.Fatal(err)
	}
	err := RejectUnsupported("provider", map[string]bool{"zeta": true, "alpha": true})
	if err == nil || !strings.Contains(err.Error(), "alpha, zeta") {
		t.Fatalf("error = %v", err)
	}
}
