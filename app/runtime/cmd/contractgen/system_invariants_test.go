package main

import "testing"

func TestSystemInvariantsReturnsCallerOwnedBoundaries(t *testing.T) {
	first := systemInvariants()
	if len(first) == 0 || len(first[0].Boundaries) == 0 {
		t.Fatal("system invariant catalog is empty")
	}
	want := first[0].Boundaries[0]
	first[0].Boundaries[0] = "changed"
	if got := systemInvariants()[0].Boundaries[0]; got != want {
		t.Fatalf("invariants leaked nested boundary ownership: got %q, want %q", got, want)
	}
}
