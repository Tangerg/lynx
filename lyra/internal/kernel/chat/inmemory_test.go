package chat_test

import (
	"context"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"

	"github.com/Tangerg/lynx/lyra/internal/domain/approval"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/domain/maintenance"
	"github.com/Tangerg/lynx/lyra/internal/kernel"
	"github.com/Tangerg/lynx/lyra/internal/kernel/chat"
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

	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
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
		case chat.TurnStart:
			if e.SessionID != "sess-1" {
				t.Errorf("TurnStart.SessionID = %q, want sess-1", e.SessionID)
			}
			if e.TurnID == "" {
				t.Error("TurnStart.TurnID is empty")
			}
		case chat.ToolCallStart:
			if e.ToolName != "bash" {
				t.Errorf("ToolCallStart.ToolName = %q, want bash", e.ToolName)
			}
			if !strings.Contains(e.Arguments, "echo lyra") {
				t.Errorf("ToolCallStart.Arguments missing command: %q", e.Arguments)
			}
		case chat.ToolCallEnd:
			if e.Err != "" {
				t.Errorf("ToolCallEnd.Err = %q, want empty", e.Err)
			}
			if !strings.Contains(e.Output, "lyra") {
				t.Errorf("ToolCallEnd.Output missing 'lyra': %q", e.Output)
			}
		case chat.MessageDelta:
			if !strings.Contains(e.Text, "lyra") {
				t.Errorf("MessageDelta.Text missing 'lyra': %q", e.Text)
			}
		case chat.TurnEnd:
			if e.Reason != chat.TurnEndCompleted {
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
	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
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

// TestService_PlanMode_ApprovePath runs the canonical plan-mode flow:
// the LLM produces a plan → service emits PlanGenerated → test
// calls Resume(true) → execution proceeds → TurnEnd(Completed).
func TestService_PlanMode_ApprovePath(t *testing.T) {
	svc, _ := buildPlanService(t, "1. read file\n2. edit it")
	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-plan",
		Message:   "do the thing",
		PlanMode:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	events, _ := svc.Events(context.Background(), handle)

	var (
		gotPlan      *chat.PlanGenerated
		gotTurnEnd   *chat.TurnEnd
		messageDelta bool
	)
	for ev := range events {
		switch e := ev.(type) {
		case chat.PlanGenerated:
			gotPlan = &e
		case chat.TurnInterrupted:
			// The turn parked on plan approval (R model). Approve it —
			// the continuation streams onto the same event channel.
			if err := svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case chat.MessageDelta:
			messageDelta = true
		case chat.TurnEnd:
			tmp := e
			gotTurnEnd = &tmp
		}
	}

	if gotPlan == nil {
		t.Fatal("PlanGenerated never fired")
	}
	if !strings.Contains(gotPlan.Plan, "read file") {
		t.Errorf("plan text missing seed: %q", gotPlan.Plan)
	}
	if !messageDelta {
		t.Error("approve path should run the actual model afterwards (no MessageDelta seen)")
	}
	if gotTurnEnd == nil || gotTurnEnd.Reason != chat.TurnEndCompleted {
		t.Errorf("turn end = %+v, want Completed", gotTurnEnd)
	}
}

// TestService_PlanMode_RejectPath verifies Reject short-circuits
// the turn without ever calling the model for the real reply.
func TestService_PlanMode_RejectPath(t *testing.T) {
	svc, stub := buildPlanService(t, "1. step")
	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-reject",
		Message:   "do",
		PlanMode:  true,
	})
	events, _ := svc.Events(context.Background(), handle)

	priorCalls := stub.callCount()
	var endReason chat.TurnEndReason
	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnInterrupted:
			_ = svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: false})
		case chat.TurnEnd:
			endReason = e.Reason
		}
	}

	if endReason != chat.TurnEndCanceled {
		t.Errorf("reject path: TurnEnd reason = %s, want canceled", endReason)
	}
	// One call total: the plan generation. The real Chat path must not run.
	if got := stub.callCount() - priorCalls; got != 1 {
		t.Errorf("stub Call count after reject: %d, want 1 (plan only)", got)
	}
}

// TestService_PlanMode_GatedWhenClientCannotHandle covers the anti-deadlock
// gate: plan-review surfaces as a "question" interrupt, so a client that
// declared only "approval" can't answer it — the plan-mode turn must
// auto-deny (reject the plan) instead of surfacing a TurnInterrupted no one
// can resolve (API.md §6.2).
func TestService_PlanMode_GatedWhenClientCannotHandle(t *testing.T) {
	svc, _ := buildPlanService(t, "1. step")
	svc.SetInterruptKinds([]string{"approval"}) // no "plan"

	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-gated",
		Message:   "do",
		PlanMode:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	events, _ := svc.Events(context.Background(), handle)

	var sawInterrupt bool
	var endReason chat.TurnEndReason
	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnInterrupted:
			sawInterrupt = true
		case chat.TurnEnd:
			endReason = e.Reason
		}
	}
	if sawInterrupt {
		t.Error("plan interrupt should have been gated (auto-denied), not surfaced")
	}
	if endReason != chat.TurnEndCanceled {
		t.Errorf("gated plan should end Canceled (rejected), got %s", endReason)
	}
}

// TestService_PlanMode_NoPlanFallthrough verifies a NO_PLAN reply
// from the LLM skips the approval prompt entirely — the turn
// runs through to a normal completion without emitting any
// PlanGenerated event.
func TestService_PlanMode_NoPlanFallthrough(t *testing.T) {
	svc, _ := buildPlanService(t, "NO_PLAN")
	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-nop",
		Message:   "two plus two?",
		PlanMode:  true,
	})
	events, _ := svc.Events(context.Background(), handle)

	for ev := range events {
		if _, ok := ev.(chat.PlanGenerated); ok {
			t.Error("NO_PLAN path should not emit PlanGenerated")
		}
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
	svc := mustChat(chat.New(eng, nil, nil))

	// Turn 1.
	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
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
	handle2, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
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
	err := svc.InjectSteering(context.Background(), chat.TurnHandle{TurnID: "no-such"}, "msg")
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
	svc := mustChat(chat.New(eng, approval.New(approval.ModeBalanced), nil)) // bash → gate

	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-approve",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		endReason    chat.TurnEndReason
	)
	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnInterrupted:
			sawInterrupt = true
			if len(e.Interrupts) != 1 || e.Interrupts[0].Kind != "approval" {
				t.Errorf("interrupts = %+v, want one approval", e.Interrupts)
			} else if p, ok := e.Interrupts[0].Payload.(chat.ApprovalPrompt); !ok || p.ToolName != "bash" {
				t.Errorf("approval payload = %+v, want bash ApprovalPrompt", e.Interrupts[0].Payload)
			}
			if err := svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case chat.TurnEnd:
			endReason = e.Reason
		}
	}
	if !sawInterrupt {
		t.Error("TurnInterrupted never fired in balanced mode")
	}
	if endReason != chat.TurnEndCompleted {
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
	svc := mustChat(chat.New(eng, approval.New(approval.ModeBalanced), nil)) // bash → gate

	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-rmodel",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var endReason chat.TurnEndReason
	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnInterrupted:
			if err := svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case chat.TurnEnd:
			endReason = e.Reason
		}
	}

	if endReason != chat.TurnEndCompleted {
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

// TestService_ApprovalGate_Deny — denying via Resume(false) makes the
// tool short-circuit with the denial fed back to the model as a
// recoverable result; the model emits its final reply and the turn
// still completes.
func TestService_ApprovalGate_Deny(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(chat.New(eng, approval.New(approval.ModeBalanced), nil))

	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-deny",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	var (
		sawDenial bool
		endReason chat.TurnEndReason
	)
	for ev := range events {
		switch e := ev.(type) {
		case chat.TurnInterrupted:
			_ = svc.Resume(context.Background(), handle, interrupts.Resolution{Approved: false})
		case chat.ToolCallEnd:
			// Denial flows back as a tool *result* so the model can
			// recover — Err stays empty, Output carries the reason.
			if strings.Contains(e.Output, "denied") {
				sawDenial = true
			}
		case chat.TurnEnd:
			endReason = e.Reason
		}
	}
	if !sawDenial {
		t.Error("expected a denied tool result after Resume(false)")
	}
	if endReason != chat.TurnEndCompleted {
		t.Errorf("turn end = %s, want completed (model recovered after denial)", endReason)
	}
}

// TestService_ApprovalGate_YoloSkipsEvent makes sure the gate is
// invisible under ModeYolo — the turn never parks (no TurnInterrupted),
// the tool runs as if no gate were wired.
func TestService_ApprovalGate_YoloSkipsEvent(t *testing.T) {
	client, _ := chatmodel.NewClient(newStubChatModel())
	eng, _ := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	svc := mustChat(chat.New(eng, approval.New(approval.ModeYolo), nil))

	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-yolo",
		Message:   "echo lyra",
	})
	events, _ := svc.Events(context.Background(), handle)

	for ev := range events {
		if _, ok := ev.(chat.TurnInterrupted); ok {
			t.Error("TurnInterrupted should NOT fire in yolo mode")
		}
	}
}

// TestService_StartTurn_Validation rejects empty SessionID / Message.
func TestService_StartTurn_Validation(t *testing.T) {
	svc, _ := buildService(t)

	if _, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{Message: "x"}); err == nil {
		t.Error("missing SessionID should error")
	}
	if _, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{SessionID: "s"}); err == nil {
		t.Error("missing Message should error")
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildService(t *testing.T) (chat.Service, *kernel.Engine) {
	t.Helper()

	client, err := chatmodel.NewClient(newStubChatModel())
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng, err := kernel.New(context.Background(), kernel.Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return mustChat(chat.New(eng, nil, nil)), eng
}

// buildPlanService stands up a service backed by a planAwareStub
// that answers "produce a plan" prompts with planText and every
// other prompt with "done". Returned alongside the stub so tests
// can assert on call counts.
func buildPlanService(t *testing.T, planText string) (chat.Service, *planAwareStub) {
	t.Helper()
	stub := newPlanAwareStub(planText, "done")
	client, err := chatmodel.NewClient(stub)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := kernel.New(context.Background(), kernel.Config{
		ChatClient: client,
		Planner:    maintenance.NewPlanner(client), // plan mode exercises the planner port
	})
	if err != nil {
		t.Fatal(err)
	}
	return mustChat(chat.New(eng, nil, nil)), stub
}

func drainEvents(events iter.Seq[chat.Event]) []chat.Event {
	var out []chat.Event
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func eventNames(events []chat.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case chat.TurnStart:
			out[i] = "TurnStart"
		case chat.MessageDelta:
			out[i] = "MessageDelta"
		case chat.ToolCallStart:
			out[i] = "ToolCallStart"
		case chat.ToolCallEnd:
			out[i] = "ToolCallEnd"
		case chat.TurnEnd:
			out[i] = "TurnEnd"
		case chat.ErrorEvent:
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

func baseSeq(ev chat.Event) uint64 {
	switch e := ev.(type) {
	case chat.TurnStart:
		return e.Seq
	case chat.MessageDelta:
		return e.Seq
	case chat.ToolCallStart:
		return e.Seq
	case chat.ToolCallEnd:
		return e.Seq
	case chat.TurnEnd:
		return e.Seq
	case chat.ErrorEvent:
		return e.Seq
	}
	return 0
}

// ------------------------------------------------------------------
// Stub model (duplicated from engine package because that one's
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

// planAwareStub routes Call/Stream on the system-prompt content:
// requests that look like plan generation (system prompt mentions
// "draft a brief plan") get planReply; everything else gets the
// regular chatReply. Tracks the total Call count so tests can
// assert on how many round-trips happened.
type planAwareStub struct {
	planReply string
	chatReply string

	defaults *chatmodel.Options
	mu       sync.Mutex
	calls    int
}

func newPlanAwareStub(planReply, chatReply string) *planAwareStub {
	opts, _ := chatmodel.NewOptions("stub-plan")
	return &planAwareStub{planReply: planReply, chatReply: chatReply, defaults: opts}
}

func (m *planAwareStub) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *planAwareStub) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *planAwareStub) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *planAwareStub) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if isPlanRequest(req) {
		return makeText(m.planReply)
	}
	return makeText(m.chatReply)
}

func (m *planAwareStub) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

// isPlanRequest returns true when the request's system prompt
// contains the plan-instructions marker. Identifying by content
// rather than a side channel keeps the stub agnostic of how the
// engine composes prompts.
func isPlanRequest(req *chatmodel.Request) bool {
	for _, msg := range req.Messages {
		if sys, ok := msg.(*chatmodel.SystemMessage); ok {
			if strings.Contains(sys.Text, "draft a brief plan") {
				return true
			}
		}
	}
	return false
}

// mustChat unwraps chat.New in test wiring — construction only fails on a
// nil engine, which tests never pass. Takes (svc, err) directly so call
// sites can splice chat.New's multi-value return straight in.
func mustChat(svc chat.Service, err error) chat.Service {
	if err != nil {
		panic(err)
	}
	return svc
}
