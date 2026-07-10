package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	runstate "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// durableRunEvent builds a durable wire RunEvent with a fixed-width eventId, for
// the SubscribeRun replay test (durability derives from the type — item.completed
// is durable).
func durableRunEvent(seq int) protocol.RunEvent {
	return protocol.RunEvent{
		EventID: fmt.Sprintf("%s%011d", protocol.IDPrefixEvent, seq),
		Event:   protocol.StreamEvent{Type: protocol.StreamItemCompleted},
	}
}

func TestOpenSegmentAfterServerCloseCancelsCreatedTurn(t *testing.T) {
	turns := &recordingTurns{}
	s := newTestServer(&stubRuntime{turns: turns})
	s.Close()
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "turn_1"}

	_, _, err := s.openSegment(context.Background(), "run_1", "", handle, handle.SessionID, nil, nil, "", "")
	if !errors.Is(err, errServerClosed) {
		t.Fatalf("openSegment err = %v, want errServerClosed", err)
	}
	if len(turns.canceled) != 1 || turns.canceled[0] != handle {
		t.Fatalf("canceled turns = %+v, want %+v", turns.canceled, handle)
	}
}

// TestSubscribeRun_StreamsLiveRunFromHub verifies the streamable-HTTP
// subscribe semantics: an actively-streaming run hands back a fresh hub
// subscription that replays the durable backlog (after Last-Event-Id when
// supplied) then tails live; anything else is run_not_found.
func TestSubscribeRun_StreamsLiveRunFromHub(t *testing.T) {
	h := runs.NewJournal[protocol.RunEvent]()
	h.Append(durableRunEvent(1))
	h.Append(durableRunEvent(2))
	s := &Server{}
	s.runs.Open(runstate.Record{ID: "run_live"}, &runHandle{hub: h})

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
	if (<-events).EventID != durableRunEvent(1).EventID || (<-events).EventID != durableRunEvent(2).EventID {
		t.Fatal("subscribe must replay the durable backlog in order")
	}

	// With Last-Event-Id (via ctx): replay only what's after it.
	ctx := transport.WithLastEventID(context.Background(), durableRunEvent(1).EventID)
	_, resumed, err := s.SubscribeRun(ctx, "run_live")
	if err != nil {
		t.Fatalf("subscribe resume: %v", err)
	}
	if (<-resumed).EventID != durableRunEvent(2).EventID {
		t.Fatal("resume must replay only events after Last-Event-Id")
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
