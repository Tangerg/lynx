package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// TestSubscribeRun_StreamsLiveRunFromHub verifies the streamable-HTTP
// subscribe semantics: an actively-streaming run hands back a fresh hub
// subscription that replays the durable backlog (after Last-Event-Id when
// supplied) then tails live; anything else is run_not_found.
func TestSubscribeRun_StreamsLiveRunFromHub(t *testing.T) {
	h := newRunHub()
	h.Append(ev(1, true))
	h.Append(ev(2, true))
	s := &Server{runs: map[string]*runEntry{"run_live": {runID: "run_live", hub: h}}}

	// From the start: replay the whole durable backlog.
	out, events, err := s.SubscribeRun(context.Background(), "run_live")
	if err != nil {
		t.Fatalf("subscribe live: %v", err)
	}
	if out == nil || out.RunID != "run_live" {
		t.Fatalf("subscribe live: out = %+v, want RunID run_live", out)
	}
	if events == nil {
		t.Fatal("subscribe must return a per-run stream")
	}
	if (<-events).EventID != ev(1, true).EventID || (<-events).EventID != ev(2, true).EventID {
		t.Fatal("subscribe must replay the durable backlog in order")
	}

	// With Last-Event-Id (via ctx): replay only what's after it.
	ctx := transport.WithLastEventID(context.Background(), ev(1, true).EventID)
	_, resumed, err := s.SubscribeRun(ctx, "run_live")
	if err != nil {
		t.Fatalf("subscribe resume: %v", err)
	}
	if (<-resumed).EventID != ev(2, true).EventID {
		t.Fatal("resume must replay only events after Last-Event-Id")
	}

	if _, _, err := s.SubscribeRun(context.Background(), "ghost"); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe unknown: err = %v, want ErrRunNotFound", err)
	}
	if _, _, err := s.SubscribeRun(context.Background(), ""); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe empty: err = %v, want ErrRunNotFound", err)
	}
}
