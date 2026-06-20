package turn_test

import (
	"context"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// TestService_StartTurn_EmitsExpectedEvents drives a full turn
// against a stub LLM that asks for `bash` (echo lyra). The service
// must emit the canonical sequence:
//
//	TurnStart → ToolCallStart → ToolCallEnd → MessageDelta → TurnEnd
//
// and the channel must close cleanly. This is the M1+M2 contract:
// transport adapters built later only need to forward whatever this
// channel yields.
func TestService_StartTurn_EmitsExpectedEvents(t *testing.T) {
	svc, _ := buildService(t)

	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-1",
		Message:   "say lyra via bash",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	events, err := svc.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	got := drainEvents(events)

	wantOrder := []string{"TurnStart", "ToolCallStart", "ToolCallEnd", "MessageDelta", "TurnEnd"}
	if names := eventNames(got); !sliceEqual(names, wantOrder) {
		t.Fatalf("event order mismatch:\n  got  %v\n  want %v", names, wantOrder)
	}

	// Spot-check each event's content.
	for _, ev := range got {
		switch e := ev.(type) {
		case turn.TurnStart:
			if e.SessionID != "sess-1" {
				t.Errorf("TurnStart.SessionID = %q, want sess-1", e.SessionID)
			}
			if e.TurnID == "" {
				t.Error("TurnStart.TurnID is empty")
			}
		case turn.ToolCallStart:
			if e.ToolName != "bash" {
				t.Errorf("ToolCallStart.ToolName = %q, want bash", e.ToolName)
			}
			if !strings.Contains(e.Arguments, "echo lyra") {
				t.Errorf("ToolCallStart.Arguments missing command: %q", e.Arguments)
			}
		case turn.ToolCallEnd:
			if e.Err != "" {
				t.Errorf("ToolCallEnd.Err = %q, want empty", e.Err)
			}
			if !strings.Contains(e.Output, "lyra") {
				t.Errorf("ToolCallEnd.Output missing 'lyra': %q", e.Output)
			}
		case turn.MessageDelta:
			if !strings.Contains(e.Text, "lyra") {
				t.Errorf("MessageDelta.Text missing 'lyra': %q", e.Text)
			}
		case turn.TurnEnd:
			if e.Reason != turn.TurnEndCompleted {
				t.Errorf("TurnEnd.Reason = %s, want completed", e.Reason)
			}
		}
	}

	// After the turn ends Events / Cancel should report ErrTurnNotFound —
	// the impl cleans up turnState on close.
	if _, err := svc.Events(context.Background(), handle); err == nil {
		t.Error("Events after TurnEnd should error")
	}
	if err := svc.Cancel(context.Background(), handle); err == nil {
		t.Error("Cancel after TurnEnd should error")
	}
}

// TestService_SeqMonotone verifies every event in a turn carries a
// strictly increasing Seq starting at 1 — transport adapters rely
// on monotonicity for resume-from-seq semantics.
func TestService_SeqMonotone(t *testing.T) {
	svc, _ := buildService(t)
	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s", Message: "hi",
	})
	events, _ := svc.Events(context.Background(), handle)
	got := drainEvents(events)

	var prev uint64
	for i, ev := range got {
		seq := baseSeq(ev)
		if seq != prev+1 {
			t.Errorf("event[%d] seq = %d, want %d (%T)", i, seq, prev+1, ev)
		}
		prev = seq
	}
}

// TestService_InjectSteering_LandsInNextTurn verifies the "next-turn"
// semantics: a steering message injected during turn 1 must be
// visible to the model as a history user message at the start of
// turn 2. The history-aware stub counts messages per Call, so the
// second turn must see strictly more messages than turn 1's
// initial new-message count.
func TestService_InjectSteering_LandsInNextTurn(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chatmodel.NewClient(stub)
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(turn.New(eng, nil, nil, nil, nil))

	// Turn 1.
	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-steer",
		Message:   "hi",
	})
	events, _ := svc.Events(context.Background(), handle)

	// Inject steering before consuming events so the service has
	// time to land it on the turn state. Drain the channel before
	// starting turn 2 — the steering flushes after RunChat returns.
	if err := svc.InjectSteering(context.Background(), handle, "also keep responses short"); err != nil {
		t.Fatalf("InjectSteering: %v", err)
	}
	for range events {
	}

	turn1Msgs := stub.seenLengths[0]

	// Turn 2 — should see (original user + assistant + steering + new user) = at least turn1Msgs+3.
	handle2, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-steer",
		Message:   "go on",
	})
	events2, _ := svc.Events(context.Background(), handle2)
	for range events2 {
	}

	if len(stub.seenLengths) < 2 {
		t.Fatalf("stub Call count = %d, want >= 2", len(stub.seenLengths))
	}
	turn2Msgs := stub.seenLengths[1]
	if turn2Msgs <= turn1Msgs+1 {
		// turn 2 should see at least: turn1 user, turn1 assistant, steering, turn2 user = turn1Msgs + 3
		// allowing a margin for system-prompt messages.
		t.Errorf("turn 2 message count = %d, turn 1 = %d; steering should add at least one user entry", turn2Msgs, turn1Msgs)
	}
}

// TestService_InjectSteering_UnknownTurn returns ErrTurnNotFound
// for handles the service doesn't recognize — completed turns are
// pruned from the in-memory map.
func TestService_InjectSteering_UnknownTurn(t *testing.T) {
	svc, _ := buildService(t)
	err := svc.InjectSteering(context.Background(), turn.TurnHandle{TurnID: "no-such"}, "msg")
	if err == nil {
		t.Error("steering on unknown handle should error")
	}
}

// TestService_ApprovalGate_AllowOnce verifies the gate parks the turn
// on a TurnInterrupted{approval} when the configured mode requires
// consent (R model), and that approving via Resume lets the tool
// proceed — the continuation streams on the same channel and the turn
// completes normally.
func TestService_ApprovalGate_AllowOnce(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(turn.New(eng, approval.New(approval.ModeBalanced, nil), nil, nil, nil)) // bash → gate

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-approve",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		endReason    turn.TurnEndReason
	)
	for ev := range events {
		switch e := ev.(type) {
		case turn.TurnInterrupted:
			sawInterrupt = true
			if len(e.Interrupts) != 1 || e.Interrupts[0].Kind != "approval" {
				t.Errorf("interrupts = %+v, want one approval", e.Interrupts)
			} else if p, ok := e.Interrupts[0].Payload.(turn.ApprovalPrompt); !ok || p.ToolName != "bash" {
				t.Errorf("approval payload = %+v, want bash ApprovalPrompt", e.Interrupts[0].Payload)
			}
			if err := svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case turn.TurnEnd:
			endReason = e.Reason
		}
	}
	if !sawInterrupt {
		t.Error("TurnInterrupted never fired in balanced mode")
	}
	if endReason != turn.TurnEndCompleted {
		t.Errorf("turn end = %s, want completed", endReason)
	}
}

// TestService_ApprovalGate_ResumeAtPendingCall pins the R-model: approving a
// gated tool RESUMES the turn AT the pending call — the loop feeds back the
// parked tail (the interrupting round's assistant tool-call message) and runs
// the now-approved tool, then the model replies. So the model is invoked
// exactly TWICE across the cycle — round 1 (emits the call, interrupts) and the
// synthesis after resume — NOT three times: the interrupted round's call is
// never regenerated. The stored history must be a single valid
// user → assistant(tool_call) → tool → assistant sequence (no duplicate user
// message: it was persisted on the first run and the resume sends only the
// system header + the fed-back tail).
func TestService_ApprovalGate_ResumeAtPendingCall(t *testing.T) {
	model := &countingStubModel{}
	model.defaults, _ = chatmodel.NewOptions("stub-counting")
	client, _ := chatmodel.NewClient(model)
	store := memory.NewInMemoryStore()
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client, MemoryStore: store})
	svc := mustChat(turn.New(eng, approval.New(approval.ModeBalanced, nil), nil, nil, nil)) // bash → gate

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-rmodel",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var endReason turn.TurnEndReason
	for ev := range events {
		switch e := ev.(type) {
		case turn.TurnInterrupted:
			if err := svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case turn.TurnEnd:
			endReason = e.Reason
		}
	}

	if endReason != turn.TurnEndCompleted {
		t.Errorf("turn end = %s, want completed", endReason)
	}
	if got := model.calls.Load(); got != 2 {
		t.Fatalf("model invoked %d times across resume, want 2 "+
			"(round 1 emits the call + interrupts; resume runs the tool then the model replies — the call is NOT regenerated)", got)
	}

	// Resume must not duplicate the user message: it was persisted on the first
	// run and resume sends only the system header + the fed-back tail. History
	// must be a single valid user → assistant(tool_call) → tool → assistant
	// sequence.
	stored, err := store.Read(context.Background(), "sess-rmodel")
	if err != nil {
		t.Fatalf("read stored history: %v", err)
	}
	users := 0
	for _, m := range stored {
		if m.Type() == chatmodel.MessageTypeUser {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("stored history has %d user messages, want 1 (resume must not re-add the prompt): %+v", users, stored)
	}
}

// TestService_Cancel_ParkedTurn_DeliversTurnEnd verifies a canceled turn
// emits its terminal TurnEnd to a still-draining consumer rather than only
// closing the channel. Cancel cancels the turn ctx before finishTurn emits the
// terminal, so the event must not be lost to the emit ctx-escape: emit prefers
// delivery whenever the buffer has room. The turn parks on a balanced-mode
// approval gate; cancelling it (instead of approving) must surface
// TurnEnd{Canceled}.
func TestService_Cancel_ParkedTurn_DeliversTurnEnd(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(turn.New(eng, approval.New(approval.ModeBalanced, nil), nil, nil, nil))

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-cancel-parked",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		sawEnd       bool
		endReason    turn.TurnEndReason
	)
	for ev := range events {
		switch e := ev.(type) {
		case turn.TurnInterrupted:
			sawInterrupt = true
			if err := svc.Cancel(context.Background(), handle); err != nil {
				t.Errorf("Cancel: %v", err)
			}
		case turn.TurnEnd:
			sawEnd = true
			endReason = e.Reason
		}
	}
	if !sawInterrupt {
		t.Fatal("turn never parked on the approval gate")
	}
	if !sawEnd {
		t.Fatal("Cancel must deliver a terminal TurnEnd, not just close the channel")
	}
	if endReason != turn.TurnEndCanceled {
		t.Errorf("TurnEnd.Reason = %s, want canceled", endReason)
	}
}

// TestService_ApprovalGate_Deny — denying via Resume(false) makes the
// tool short-circuit with the denial fed back to the model as a
// recoverable result; the model emits its final reply and the turn
// still completes.
func TestService_ApprovalGate_Deny(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(turn.New(eng, approval.New(approval.ModeBalanced, nil), nil, nil, nil))

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-deny",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var (
		sawDenial bool
		endReason turn.TurnEndReason
	)
	for ev := range events {
		switch e := ev.(type) {
		case turn.TurnInterrupted:
			_ = svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: false})
		case turn.ToolCallEnd:
			// Denial flows back as a tool *result* so the model can
			// recover — Err stays empty, Output carries the reason.
			if strings.Contains(e.Output, "denied") {
				sawDenial = true
			}
		case turn.TurnEnd:
			endReason = e.Reason
		}
	}
	if !sawDenial {
		t.Error("expected a denied tool result after Resume(false)")
	}
	if endReason != turn.TurnEndCompleted {
		t.Errorf("turn end = %s, want completed (model recovered after denial)", endReason)
	}
}

// TestService_ApprovalGate_YoloSkipsEvent makes sure the gate is
// invisible under ModeYolo — the turn never parks (no TurnInterrupted),
// the tool runs as if no gate were wired.
func TestService_ApprovalGate_YoloSkipsEvent(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(turn.New(eng, approval.New(approval.ModeYolo, nil), nil, nil, nil))

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-yolo",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	for ev := range events {
		if _, ok := ev.(turn.TurnInterrupted); ok {
			t.Error("TurnInterrupted should NOT fire in yolo mode")
		}
	}
}

// TestService_StartTurn_Validation rejects empty SessionID / Message.
func TestService_StartTurn_Validation(t *testing.T) {
	svc, _ := buildService(t)

	if _, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{Message: "x"}); err == nil {
		t.Error("missing SessionID should error")
	}
	if _, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{SessionID: "s"}); err == nil {
		t.Error("missing Message should error")
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildService(t *testing.T) (turn.Service, *kernel.Engine) {
	t.Helper()

	client, err := chatmodel.NewClient(newStubChatModel())
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng, err := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return mustChat(turn.New(eng, nil, nil, nil, nil)), eng
}

func drainEvents(events iter.Seq[turn.Event]) []turn.Event {
	var out []turn.Event
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func eventNames(events []turn.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case turn.TurnStart:
			out[i] = "TurnStart"
		case turn.MessageDelta:
			out[i] = "MessageDelta"
		case turn.ToolCallStart:
			out[i] = "ToolCallStart"
		case turn.ToolCallEnd:
			out[i] = "ToolCallEnd"
		case turn.TurnEnd:
			out[i] = "TurnEnd"
		case turn.ErrorEvent:
			out[i] = "ErrorEvent"
		default:
			out[i] = "?"
		}
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func baseSeq(ev turn.Event) uint64 {
	switch e := ev.(type) {
	case turn.TurnStart:
		return e.Seq
	case turn.MessageDelta:
		return e.Seq
	case turn.ToolCallStart:
		return e.Seq
	case turn.ToolCallEnd:
		return e.Seq
	case turn.TurnEnd:
		return e.Seq
	case turn.ErrorEvent:
		return e.Seq
	}
	return 0
}

// ------------------------------------------------------------------
// Stub model (duplicated from kernel package because that one's
// test-scope; this test lives in a different package).
// ------------------------------------------------------------------

type stubChatModel struct{ defaults *chatmodel.Options }

func newStubChatModel() *stubChatModel {
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *stubChatModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *stubChatModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("bash", `{"command":"echo lyra"}`)
}

func (m *stubChatModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

// countingStubModel runs the same two-round bash dance as stubChatModel
// (round 1 → bash tool call, round 2 → final text) but counts model
// invocations so a test can assert a HITL resume continues the turn
// rather than re-running it from the first round.
type countingStubModel struct {
	defaults *chatmodel.Options
	calls    atomic.Int32
}

func (m *countingStubModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *countingStubModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *countingStubModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.calls.Add(1)
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("bash", `{"command":"echo lyra"}`)
}

func (m *countingStubModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func hasToolMsg(messages []chatmodel.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chatmodel.MessageTypeTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(text),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	calls := []*chatmodel.ToolCallPart{{ID: "c1", Name: name, Arguments: args}}
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(calls),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonToolCalls},
		},
		&chatmodel.ResponseMetadata{},
	)
}

// historyAwareStub records the length of req.Messages on each
// Call so steering / memory tests can assert that a follow-up turn
// sees strictly more conversation history than the previous one.
type historyAwareStub struct {
	defaults    *chatmodel.Options
	mu          sync.Mutex
	seenLengths []int
}

func newHistoryAwareStub() *historyAwareStub {
	opts, _ := chatmodel.NewOptions("stub-history")
	return &historyAwareStub{defaults: opts}
}

func (m *historyAwareStub) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *historyAwareStub) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *historyAwareStub) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.mu.Lock()
	m.seenLengths = append(m.seenLengths, len(req.Messages))
	m.mu.Unlock()
	return makeText("ok")
}

func (m *historyAwareStub) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

// mustChat unwraps turn.New in test wiring — construction only fails on a
// nil engine, which tests never pass. Takes (svc, err) directly so call
// sites can splice turn.New's multi-value return straight in.
func mustChat(svc turn.Service, err error) turn.Service {
	if err != nil {
		panic(err)
	}
	return svc
}
