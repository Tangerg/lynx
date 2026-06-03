package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// TestSubscribeRun_AcksLiveRunOnly verifies the single-tenant subscribe
// semantics: an actively-streaming run is acked (nil EventStream —
// delivery rides the shared SSE stream); anything else is run_not_found.
func TestSubscribeRun_AcksLiveRunOnly(t *testing.T) {
	s := &Server{runs: map[string]*runEntry{"run_live": {runID: "run_live"}}}

	out, events, err := s.SubscribeRun(context.Background(), "run_live")
	if err != nil {
		t.Fatalf("subscribe live: %v", err)
	}
	if out == nil || out.RunID != "run_live" {
		t.Fatalf("subscribe live: out = %+v, want RunID run_live", out)
	}
	if events != nil {
		t.Fatal("subscribe should not return a per-subscriber stream (single-tenant)")
	}

	if _, _, err := s.SubscribeRun(context.Background(), "ghost"); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe unknown: err = %v, want ErrRunNotFound", err)
	}
	if _, _, err := s.SubscribeRun(context.Background(), ""); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe empty: err = %v, want ErrRunNotFound", err)
	}
}
