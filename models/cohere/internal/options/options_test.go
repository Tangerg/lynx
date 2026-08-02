package options

import (
	"strconv"
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

func TestInt(t *testing.T) {
	if got, err := Int("field", 42); err != nil || got != 42 {
		t.Fatalf("Int(42) = %d, %v", got, err)
	}
	if strconv.IntSize == 32 {
		if _, err := Int("field", int64(1<<31)); err == nil {
			t.Fatal("Int accepted a value outside the platform int range")
		}
	}
}
