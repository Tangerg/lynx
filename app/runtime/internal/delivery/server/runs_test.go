package server

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// TestSubscribeRun_AttachesToTheAddressedSegment verifies the vNext subscribe
// semantics: the request names a segment, the ack names the position the tail
// starts after, and a request that addresses something else is refused by name.
//
// A cursorless subscribe is deliberately NOT a replay: the opening events are
// already published, so this stream carries only what comes next, and the client
// recovers the rest from items.list.
func TestSubscribeRun_AttachesToTheAddressedSegment(t *testing.T) {
	s := newBlockingServer(t)
	runID, segmentID := startLiveRun(t, s, t.TempDir())

	out, events, err := s.SubscribeRun(context.Background(), protocol.SubscribeRunRequest{
		RunID: runID, SegmentID: segmentID,
	})
	if err != nil {
		t.Fatalf("subscribe live: %v", err)
	}
	if out == nil || out.RunID != runID || out.SegmentID != segmentID || events == nil {
		t.Fatalf("subscribe live: out=%+v events=%v", out, events)
	}
	// The opening events are already on the stream, so the ack must name a head —
	// and it must be framed like every other event id the client sees.
	if !strings.HasPrefix(out.HeadEventID, protocol.IDPrefixEvent) {
		t.Fatalf("headEventId = %q, want an evt_-framed position", out.HeadEventID)
	}

	// A segment the run is not executing: the client is holding a replaced
	// execution, and attaching it to the live one would corrupt its fold silently.
	if _, _, err := s.SubscribeRun(context.Background(), protocol.SubscribeRunRequest{
		RunID: runID, SegmentID: "seg_replaced",
	}); !errors.Is(err, protocol.ErrStaleSegment) {
		t.Fatalf("subscribe stale segment: err = %v, want ErrStaleSegment", err)
	}
	if _, _, err := s.SubscribeRun(context.Background(), protocol.SubscribeRunRequest{
		RunID: "ghost", SegmentID: segmentID,
	}); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("subscribe unknown: err = %v, want ErrRunNotFound", err)
	}
}

// A steer addresses the same live segment a subscribe does, and refuses the same
// way. Before this, every one of these was run_not_found — which told the client
// to go looking for a run that was right there.
func TestSteerRun_RefusesASegmentTheRunIsNotExecuting(t *testing.T) {
	s := newBlockingServer(t)
	runID, _ := startLiveRun(t, s, t.TempDir())

	if err := s.SteerRun(context.Background(), protocol.SteerRunRequest{
		RunID: runID, ExpectedSegmentID: "seg_replaced",
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "wait"}},
	}); !errors.Is(err, protocol.ErrStaleSegment) {
		t.Fatalf("steer stale segment: err = %v, want ErrStaleSegment", err)
	}
	if err := s.SteerRun(context.Background(), protocol.SteerRunRequest{
		RunID: "ghost", ExpectedSegmentID: "seg_1",
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "wait"}},
	}); !errors.Is(err, protocol.ErrRunNotFound) {
		t.Fatalf("steer unknown run: err = %v, want ErrRunNotFound", err)
	}

}

func TestWireLiveSegmentErrorPreservesTheInvalidInputField(t *testing.T) {
	_, err := decodeRunInput([]protocol.ContentBlock{{
		Type: protocol.ContentBlockImage, Mime: "image/png", Data: "not-base64",
	}})
	constraint, ok := errors.AsType[*protocol.ConstraintError](err)
	if !ok || !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("steer invalid image: err = %v, want typed invalid_params", err)
	}
	if len(constraint.Fields) != 1 ||
		constraint.Fields[0].Field != "input[0].data" ||
		constraint.Fields[0].Detail != "must be valid base64" {
		t.Fatalf("steer invalid image fields = %+v", constraint.Fields)
	}
}

func TestRunInputFromWireReportsTheInvalidBlockField(t *testing.T) {
	_, err := decodeRunInput([]protocol.ContentBlock{{
		Type: protocol.ContentBlockImage,
		Mime: "image/png",
	}})
	constraint, ok := errors.AsType[*protocol.ConstraintError](err)
	if !ok || !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("error = %v, want typed invalid_params ConstraintError", err)
	}
	if len(constraint.Fields) != 1 ||
		constraint.Fields[0].Field != "input[0].data" ||
		constraint.Fields[0].Detail != "is required for image content" {
		t.Fatalf("fields = %+v", constraint.Fields)
	}
}

// TestStartCommandMaterializeInput covers the Application-owned conversion from
// canonical input blocks to executor media and opening text.
func TestStartCommandMaterializeInput(t *testing.T) {
	imageBytes := []byte("semantic image bytes")

	// text + image: text joins, one media with decoded bytes carried through.
	text, imgs, _, err := (runs.StartCommand{Input: []transcript.ContentBlock{
		{Kind: transcript.TextContent, Text: "look at"},
		{Kind: transcript.TextContent, Text: "this"},
		{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: imageBytes},
	}}).MaterializeInput()
	if err != nil {
		t.Fatalf("text+image: %v", err)
	}
	if text != "look at\nthis" {
		t.Fatalf("text = %q, want joined", text)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 media, got %d", len(imgs))
	}
	if got, err := imgs[0].Bytes(); err != nil || !bytes.Equal(got, imageBytes) {
		t.Fatalf("media data = %q, %v; want bytes %q", got, err, imageBytes)
	}
	if imgs[0].MIME != "image/png" {
		t.Fatalf("media mime = %q, want image/png", imgs[0].MIME)
	}

	// image-only: no text is fine (the StartRun guard accepts media-only).
	if text, imgs, _, err := (runs.StartCommand{Input: []transcript.ContentBlock{
		{Kind: transcript.ImageContent, MediaType: "image/jpeg", Bytes: imageBytes},
	}}).MaterializeInput(); err != nil || text != "" || len(imgs) != 1 {
		t.Fatalf("image-only: text=%q imgs=%d err=%v", text, len(imgs), err)
	}

	// A non-image mime, an unparseable mime, and empty data are all rejected.
	if _, _, _, err := (runs.StartCommand{Input: []transcript.ContentBlock{
		{Kind: transcript.ImageContent, MediaType: "text/plain", Bytes: imageBytes},
	}}).MaterializeInput(); !errors.Is(err, runs.ErrUnsupportedMedia) {
		t.Fatalf("non-image mime: err = %v, want ErrUnsupportedMedia", err)
	}
	if _, _, _, err := (runs.StartCommand{Input: []transcript.ContentBlock{
		{Kind: transcript.ImageContent, MediaType: "not-a-mime", Bytes: imageBytes},
	}}).MaterializeInput(); !errors.Is(err, runs.ErrUnsupportedMedia) {
		t.Fatalf("bad mime: err = %v, want ErrUnsupportedMedia", err)
	}
	if _, _, _, err := (runs.StartCommand{Input: []transcript.ContentBlock{
		{Kind: transcript.ImageContent, MediaType: "image/png"},
	}}).MaterializeInput(); !errors.Is(err, runs.ErrUnsupportedMedia) {
		t.Fatalf("empty data: err = %v, want ErrUnsupportedMedia", err)
	}
}

// TestStartRunCarriesOneLimitsValueToTheDurableRun closes the delivery-to-
// application boundary: the wire names a cumulative run allowance, and the
// durable Run must expose that exact allowance. In particular,
// maxTotalTokens must never be confused with params.maxTokens, which is only a
// per-model-call output cap.
func TestStartRunCarriesOneLimitsValueToTheDurableRun(t *testing.T) {
	s := newBlockingServer(t)
	sess, err := s.sessions.CreateView(t.Context(), "", t.TempDir())
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started, _, err := s.StartRun(t.Context(), protocol.StartRunRequest{
		SessionID:      sess.ID,
		Input:          []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "measure this run"}},
		MaxTotalTokens: 12_345,
		MaxSteps:       17,
		MaxBudgetUSD:   2.75,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	t.Cleanup(s.Close)

	got, err := s.GetRun(t.Context(), protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatalf("get started run: %v", err)
	}
	want := protocol.RunLimits{MaxTotalTokens: 12_345, MaxSteps: 17, MaxBudgetUSD: 2.75}
	if got.Limits == nil || *got.Limits != want {
		t.Fatalf("durable limits = %+v, want %+v", got.Limits, want)
	}
}

func TestWireTurnStartErrMapsInvalidTurnLimit(t *testing.T) {
	err := wireRunStartErr(runs.ErrInvalidRunLimit)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
	if !errors.Is(err, runs.ErrInvalidRunLimit) {
		t.Fatalf("err = %v, want original ErrInvalidTurnLimit", err)
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
	err := wireRunStartErr(runs.ErrInvalidRunOptions)
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
	if !errors.Is(err, runs.ErrInvalidRunOptions) {
		t.Fatalf("err = %v, want original ErrInvalidTurnOptions", err)
	}
}

// TestDecodeResumeResponses covers the wire DTO → application response-union
// mapping. Durable item coverage and schema validation run later in
// application/runs against the actual open interrupt.
func TestDecodeResumeResponses(t *testing.T) {
	approval := func(v protocol.InterruptResponseValue) []protocol.InterruptResponse {
		v.Type = "approval"
		return []protocol.InterruptResponse{{ItemID: "item_1", Response: v}}
	}

	// approve + editedArgs + remember{session}: approved, args marshaled, scope carried.
	responses, err := decodeResumeResponses(approval(protocol.InterruptResponseValue{
		Decision:   "approve",
		EditedArgs: map[string]any{"cmd": "ls -la"},
		Remember:   &protocol.RememberScope{Scope: "session"},
	}))
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	res := responses[0]
	if res.ItemID != "item_1" || res.Approval == nil || !res.Approval.Approved ||
		res.Approval.RememberScope != "session" || res.Approval.Arguments != `{"cmd":"ls -la"}` {
		t.Fatalf("approve = %+v, want approved+remember{session}+args", res)
	}

	// deny + remember{session}: a remembered denial is valid.
	responses, _ = decodeResumeResponses(approval(protocol.InterruptResponseValue{
		Decision: "deny",
		Remember: &protocol.RememberScope{Scope: "session"},
	}))
	if res := responses[0].Approval; res == nil || res.Approved || res.RememberScope != "session" {
		t.Fatalf("deny+remember = %+v, want !approved && scope=session", res)
	}

	// project / global scopes are now honored (persisted as rules), carried verbatim.
	responses, _ = decodeResumeResponses(approval(protocol.InterruptResponseValue{
		Decision: "approve",
		Remember: &protocol.RememberScope{Scope: "global"},
	}))
	if scope := responses[0].Approval.RememberScope; scope != "global" {
		t.Fatalf("scope=global = %q, want carried verbatim", scope)
	}

	// No remember directive → empty scope (don't persist).
	responses, _ = decodeResumeResponses(approval(protocol.InterruptResponseValue{Decision: "approve"}))
	if scope := responses[0].Approval.RememberScope; scope != "" {
		t.Fatalf("no-remember = %q, want empty scope", scope)
	}

	// Bad decision → error.
	if _, err := decodeResumeResponses(approval(protocol.InterruptResponseValue{Decision: "maybe"})); err == nil {
		t.Fatal("decision=maybe must be an error")
	}

	// Empty stays empty; application validation rejects missing coverage.
	if responses, err := decodeResumeResponses(nil); err != nil || len(responses) != 0 {
		t.Fatalf("empty decode = %+v, %v", responses, err)
	}
}
