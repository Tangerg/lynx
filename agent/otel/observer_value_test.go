package otel

import (
	"math"
	"testing"
)

func TestUint64AttributePreservesFullRange(t *testing.T) {
	attribute := uint64Attribute("agent.test.sequence", math.MaxUint64)
	if got := attribute.Value.AsString(); got != "18446744073709551615" {
		t.Fatalf("uint64 attribute = %q", got)
	}
}

func TestSaturatingInt64PreservesRepresentableCounts(t *testing.T) {
	if got := saturatingInt64(42); got != 42 {
		t.Fatalf("representable count = %d", got)
	}
	if got := saturatingInt64(math.MaxUint64); got != math.MaxInt64 {
		t.Fatalf("overflowing count = %d, want %d", got, int64(math.MaxInt64))
	}
}
