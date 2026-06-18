package server

import (
	"strings"
	"testing"
)

// TestNextEventID_MonotonicAndFormatted verifies the server's global
// RunEvent id source is strictly increasing and matches the contract
// format evt_<zero-padded-decimal> (TRANSPORT.md §9.1). The fixed width
// makes lexical comparison agree with numeric, which the SSE replay path
// relies on.
func TestNextEventID_MonotonicAndFormatted(t *testing.T) {
	s := &Server{}

	first := s.nextEventID()
	if !strings.HasPrefix(first, "evt_") {
		t.Fatalf("eventId %q missing evt_ prefix", first)
	}
	if first != "evt_00000000001" {
		t.Fatalf("first eventId = %q, want evt_00000000001", first)
	}

	prev := first
	for range 20 {
		next := s.nextEventID()
		if next <= prev { // fixed-width padding → lexical order == numeric order
			t.Fatalf("eventId not strictly increasing: %q then %q", prev, next)
		}
		prev = next
	}
}
