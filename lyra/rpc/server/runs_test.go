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

// TestResolveResolution covers the approval-response → InterruptResolution
// mapping the resume path depends on (B5): approve/deny, editedArgs marshaled
// into the one-shot Arguments override, and remember{scope} honored only for
// "session". An unknown decision is invalid; an empty response continues.
func TestResolveResolution(t *testing.T) {
	approval := func(v protocol.InterruptResponseValue) []protocol.InterruptResponse {
		v.Type = "approval"
		return []protocol.InterruptResponse{{Response: v}}
	}

	// approve + editedArgs + remember{session}: approved, args marshaled, remembered.
	res, err := resolveResolution(approval(protocol.InterruptResponseValue{
		Decision:   "approve",
		EditedArgs: map[string]any{"cmd": "ls -la"},
		Remember:   &protocol.RememberScope{Scope: "session"},
	}))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !res.Approved || !res.Remember || res.Arguments != `{"cmd":"ls -la"}` {
		t.Fatalf("approve = %+v, want approved+remember+args", res)
	}

	// deny + remember{session}: a remembered denial is valid.
	res, _ = resolveResolution(approval(protocol.InterruptResponseValue{
		Decision: "deny",
		Remember: &protocol.RememberScope{Scope: "session"},
	}))
	if res.Approved || !res.Remember {
		t.Fatalf("deny+remember = %+v, want !approved && remember", res)
	}

	// Unhonored scope (not "session") is one-shot, not a false promise.
	res, _ = resolveResolution(approval(protocol.InterruptResponseValue{
		Decision: "approve",
		Remember: &protocol.RememberScope{Scope: "global"},
	}))
	if res.Remember {
		t.Fatal("scope=global must not set Remember (not persisted in v1)")
	}

	// Bad decision → error.
	if _, err := resolveResolution(approval(protocol.InterruptResponseValue{Decision: "maybe"})); err == nil {
		t.Fatal("decision=maybe must be an error")
	}

	// No actionable response → continue (approved).
	if res, _ := resolveResolution(nil); !res.Approved {
		t.Fatal("empty responses must continue (approved)")
	}
}
