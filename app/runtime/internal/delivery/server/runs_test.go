package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestSubscribeRun_StreamsLiveRun verifies the streamable-HTTP subscribe
// semantics: an actively-streaming run hands back a fresh subscription that
// replays its backlog then tails live; anything else is run_not_found. (Backlog
// replay / Last-Event-Id windowing is covered by the Journal's own tests.)
func TestSubscribeRun_StreamsLiveRun(t *testing.T) {
	s := newTestServer(&blockingRunRuntime{})
	startLiveRun(t, s, "run_live")

	out, events, err := s.SubscribeRun(context.Background(), "run_live")
	if err != nil {
		t.Fatalf("subscribe live: %v", err)
	}
	if out == nil || out.RunID != "run_live" || events == nil {
		t.Fatalf("subscribe live: out=%+v events=%v", out, events)
	}
	// The live run's opening run.started is durable, so a fresh subscription
	// replays it.
	select {
	case ev := <-events:
		if ev.RunID != "run_live" {
			t.Fatalf("first event runId = %q, want run_live", ev.RunID)
		}
	case <-time.After(time.Second):
		t.Fatal("subscribe must replay the live run's opening event")
	}

	if _, _, err := s.SubscribeRun(context.Background(), "ghost"); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe unknown: err = %v, want ErrRunNotFound", err)
	}
	if _, _, err := s.SubscribeRun(context.Background(), ""); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe empty: err = %v, want ErrRunNotFound", err)
	}
}

// TestCollectUserInput covers the wire-input → (text, media) split that
// runs.start feeds the turn: text blocks join, image blocks become core media
// carrying the parsed mime + base64 data, and a malformed image block is a
// hard error (not silently dropped) so a bad attachment surfaces to the user.
func TestCollectUserInput(t *testing.T) {
	const b64 = "iVBORw0KGgoAAAANSUhEUg=="

	// text + image: text joins, one media with mime + base64 carried through.
	text, imgs, err := collectUserInput([]protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: "look at"},
		{Type: protocol.ContentBlockText, Text: "this"},
		{Type: protocol.ContentBlockImage, Mime: "image/png", Data: b64},
	})
	if err != nil {
		t.Fatalf("text+image: %v", err)
	}
	if text != "look at\nthis" {
		t.Fatalf("text = %q, want joined", text)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 media, got %d", len(imgs))
	}
	if got, _ := imgs[0].DataAsString(); got != b64 {
		t.Fatalf("media data = %q, want raw base64", got)
	}
	if imgs[0].MimeType.TypeAndSubType() != "image/png" {
		t.Fatalf("media mime = %q, want image/png", imgs[0].MimeType.TypeAndSubType())
	}

	// image-only: no text is fine (the StartRun guard accepts media-only).
	if text, imgs, err := collectUserInput([]protocol.ContentBlock{
		{Type: protocol.ContentBlockImage, Mime: "image/jpeg", Data: b64},
	}); err != nil || text != "" || len(imgs) != 1 {
		t.Fatalf("image-only: text=%q imgs=%d err=%v", text, len(imgs), err)
	}

	// A non-image mime, an unparseable mime, and empty data are all rejected.
	if _, _, err := collectUserInput([]protocol.ContentBlock{
		{Type: protocol.ContentBlockImage, Mime: "text/plain", Data: b64},
	}); !errors.Is(err, protocol.ErrUnsupportedMime) {
		t.Fatalf("non-image mime: err = %v, want ErrUnsupportedMime", err)
	}
	if _, _, err := collectUserInput([]protocol.ContentBlock{
		{Type: protocol.ContentBlockImage, Mime: "not-a-mime", Data: b64},
	}); !errors.Is(err, protocol.ErrUnsupportedMime) {
		t.Fatalf("bad mime: err = %v, want ErrUnsupportedMime", err)
	}
	if _, _, err := collectUserInput([]protocol.ContentBlock{
		{Type: protocol.ContentBlockImage, Mime: "image/png", Data: ""},
	}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("empty data: err = %v, want ErrInvalidParams", err)
	}
}

func TestWireTurnStartErrMapsInvalidTurnLimit(t *testing.T) {
	err := wireTurnStartErr(turn.ErrInvalidTurnLimit)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
}

func TestGenerationOptionsFromWire(t *testing.T) {
	temp := 0.7
	maxTokens := int64(1024)
	topP := 0.9
	params := &protocol.GenerationParams{
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
		Stop:        []string{"END"},
	}
	opts := generationOptionsFromWire(params)
	params.Stop[0] = "mutated"

	if opts == nil || opts.Temperature == nil || *opts.Temperature != 0.7 {
		t.Fatalf("Temperature = %v, want 0.7", opts)
	}
	if opts.MaxTokens == nil || *opts.MaxTokens != 1024 {
		t.Fatalf("MaxTokens = %v, want 1024", opts.MaxTokens)
	}
	if opts.TopP == nil || *opts.TopP != 0.9 {
		t.Fatalf("TopP = %v, want 0.9", opts.TopP)
	}
	if len(opts.Stop) != 1 || opts.Stop[0] != "END" {
		t.Fatalf("Stop = %v, want cloned END", opts.Stop)
	}
}

func TestWireTurnStartErrMapsInvalidTurnOptions(t *testing.T) {
	err := wireTurnStartErr(turn.ErrInvalidTurnOptions)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
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

	// approve + editedArgs + remember{session}: approved, args marshaled, scope carried.
	res, err := resolveResolution(approval(protocol.InterruptResponseValue{
		Decision:   "approve",
		EditedArgs: map[string]any{"cmd": "ls -la"},
		Remember:   &protocol.RememberScope{Scope: "session"},
	}))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if !res.Approved || res.RememberScope != "session" || res.Arguments != `{"cmd":"ls -la"}` {
		t.Fatalf("approve = %+v, want approved+remember{session}+args", res)
	}

	// deny + remember{session}: a remembered denial is valid.
	res, _ = resolveResolution(approval(protocol.InterruptResponseValue{
		Decision: "deny",
		Remember: &protocol.RememberScope{Scope: "session"},
	}))
	if res.Approved || res.RememberScope != "session" {
		t.Fatalf("deny+remember = %+v, want !approved && scope=session", res)
	}

	// project / global scopes are now honored (persisted as rules), carried verbatim.
	res, _ = resolveResolution(approval(protocol.InterruptResponseValue{
		Decision: "approve",
		Remember: &protocol.RememberScope{Scope: "global"},
	}))
	if res.RememberScope != "global" {
		t.Fatalf("scope=global = %q, want carried verbatim", res.RememberScope)
	}

	// No remember directive → empty scope (don't persist).
	res, _ = resolveResolution(approval(protocol.InterruptResponseValue{Decision: "approve"}))
	if res.RememberScope != "" {
		t.Fatalf("no-remember = %q, want empty scope", res.RememberScope)
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
