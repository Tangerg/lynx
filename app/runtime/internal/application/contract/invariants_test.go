package contract

import "testing"

func TestSystemInvariantsReturnsCallerOwnedBoundaries(t *testing.T) {
	first := SystemInvariants()
	if len(first) == 0 || len(first[0].Boundaries) == 0 {
		t.Fatal("system invariant catalog is empty")
	}
	want := first[0].Boundaries[0]
	first[0].Boundaries[0] = BoundarySessionDelete

	second := SystemInvariants()
	if second[0].Boundaries[0] != want {
		t.Fatalf("SystemInvariants leaked nested boundary ownership: got %q, want %q", second[0].Boundaries[0], want)
	}
}
