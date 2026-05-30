package server

import (
	"testing"

	memsvc "github.com/Tangerg/lynx/lyra/internal/service/memory"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestMemScopeRoundTrip pins the wire(string) ↔ service(int) scope
// bridge — the one spot memory.* could silently map project↔user wrong.
func TestMemScopeRoundTrip(t *testing.T) {
	cases := []struct {
		wire protocol.MemoryScope
		svc  memsvc.Scope
	}{
		{protocol.MemoryScopeProject, memsvc.ScopeProject},
		{protocol.MemoryScopeUser, memsvc.ScopeUser},
	}
	for _, c := range cases {
		if got := memScopeFromWire(c.wire); got != c.svc {
			t.Errorf("memScopeFromWire(%q) = %v, want %v", c.wire, got, c.svc)
		}
		if got := memScopeToWire(c.svc); got != c.wire {
			t.Errorf("memScopeToWire(%v) = %q, want %q", c.svc, got, c.wire)
		}
	}
}

// TestMemoryScopeValid guards the dispatch-boundary validation that
// turns a bad scope into -32602 invalid_params.
func TestMemoryScopeValid(t *testing.T) {
	for _, s := range []protocol.MemoryScope{protocol.MemoryScopeProject, protocol.MemoryScopeUser} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []protocol.MemoryScope{"", "global", "Project", "USER"} {
		if s.Valid() {
			t.Errorf("%q should be invalid", s)
		}
	}
}
