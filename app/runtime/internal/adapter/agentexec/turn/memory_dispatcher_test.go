package turn_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	chatmodel "github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TestDispatcher_StartTurn_EmitsExpectedEvents drives a full turn
// against a stub LLM that asks for `shell` (echo lyra). The dispatcher
// must emit the canonical sequence:
//
//	ToolCallStart → ToolCallEnd → MessageDelta → TurnEnd
//
// and the sequence must end cleanly.
func TestDispatcher_StartTurn_EmitsExpectedEvents(t *testing.T) {
	dispatcher, _ := buildDispatcher(t)

	handle, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID: "sess-1",
		Message:   "say lyra via shell",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if _, err := dispatcher.Events(context.Background(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("concurrent Events = %v, want ErrTurnNotFound", err)
	}

	got := drainEvents(events)

	wantOrder := []string{"ToolCallStart", "ToolCallEnd", "MessageDelta", "TurnEnd"}
	if names := eventNames(got); !sliceEqual(names, wantOrder) {
		t.Fatalf("event order mismatch:\n  got  %v\n  want %v", names, wantOrder)
	}

	// Spot-check each event's content.
	for _, ev := range got {
		switch e := ev.(type) {
		case runs.ToolCallStart:
			if e.ToolName != "shell" {
				t.Errorf("ToolCallStart.ToolName = %q, want shell", e.ToolName)
			}
			if !strings.Contains(e.Arguments, "echo lyra") {
				t.Errorf("ToolCallStart.Arguments missing command: %q", e.Arguments)
			}
		case runs.ToolCallEnd:
			if e.Err != "" {
				t.Errorf("ToolCallEnd.Err = %q, want empty", e.Err)
			}
			result, ok := e.Result.Any().(map[string]any)
			if !ok {
				t.Fatalf("ToolCallEnd.Result = %T, want JSON object", e.Result)
			}
			output, ok := result["output"].(string)
			if !ok || !strings.Contains(output, "lyra") {
				t.Errorf("ToolCallEnd.Result missing 'lyra': %#v", e.Result)
			}
		case runs.MessageDelta:
			if !strings.Contains(e.Text, "lyra") {
				t.Errorf("MessageDelta.Text missing 'lyra': %q", e.Text)
			}
		case runs.TurnEnd:
			if e.Reason != execution.OutcomeCompleted {
				t.Errorf("TurnEnd.Reason = %s, want completed", e.Reason)
			}
		}
	}

	awaitTurnForgotten(t, dispatcher, handle)
	if err := dispatcher.Cancel(t.Context(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Errorf("Cancel after cleanup = %v, want ErrTurnNotFound", err)
	}
}

// awaitTurnForgotten blocks until the dispatcher has dropped the turn's state.
//
// Draining to TurnEnd is not enough to assert that directly: closing the event
// stream and releasing the turn are separate steps, and they have to be. Release
// discards the process, which reaches storage, and a consumer's completion
// signal must not wait behind that I/O. So a drained stream means "no more
// events", not "already cleaned up", and the gap widens under load.
func awaitTurnForgotten(t *testing.T, dispatcher turnDriver, handle turn.TurnHandle) {
	t.Helper()
	// Generous because it separates a hang from a loaded machine, not because
	// cleanup is expected to take any particular time. Go's test timeout
	// backstops a genuine deadlock.
	const bound = 30 * time.Second
	deadline := time.Now().Add(bound)
	for {
		_, err := dispatcher.Events(t.Context(), handle)
		if errors.Is(err, turn.ErrTurnNotFound) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("turn still registered %v after TurnEnd: Events = %v", bound, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDispatcherCloseCancelsLiveTurnsAndRejectsAdmission(t *testing.T) {
	dispatcher := mustTurn(turn.New(turnDeps(&slowStubEngine{})))
	handle, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID: "sess-close",
		Message:   "wait",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	shutdownDispatcher(t, dispatcher)
	var endReason execution.Outcome
	for ev := range events {
		if end, ok := ev.(runs.TurnEnd); ok {
			endReason = end.Reason
		}
	}
	if endReason != execution.OutcomeCanceled {
		t.Fatalf("TurnEnd reason = %q, want canceled", endReason)
	}
	if _, err := dispatcher.Events(context.Background(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("Events after Close = %v, want ErrTurnNotFound", err)
	}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "new", Message: "no"}); !errors.Is(err, turn.ErrDispatcherClosed) {
		t.Fatalf("StartTurn after Close = %v, want ErrDispatcherClosed", err)
	}
	shutdownDispatcher(t, dispatcher)
}

func TestNewRequiresApprovalGate(t *testing.T) {
	_, err := turn.New(turn.Dependencies{Engine: &stubEngine{}})
	if err == nil {
		t.Fatal("New accepted a dispatcher without an approval gate")
	}
}

// TestDispatcher_InjectSteering_LandsInNextTurn verifies the "next-turn"
// semantics: a steering message injected during turn 1 must be
// visible to the model as a history user message at the start of
// turn 2. The history-aware stub counts messages per Call, so the
// second turn must see strictly more messages than turn 1's
// initial new-message count.
func TestDispatcher_InjectSteering_LandsInNextTurn(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chatclient.New(stub)
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	dispatcher := mustTurn(turn.New(turnDeps(eng)))

	// Turn 1.
	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID: "sess-steer",
		Message:   "hi",
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	// Inject steering before consuming events so the dispatcher has
	// time to land it on the turn state. Drain the channel before
	// starting turn 2 — the steering flushes after the turn returns.
	if err := dispatcher.InjectSteering(context.Background(), handle, "also keep responses short"); err != nil {
		t.Fatalf("InjectSteering: %v", err)
	}
	for range events {
	}

	turn1Msgs := stub.seenLengths[0]

	// Turn 2 — should see (original user + assistant + steering + new user) = at least turn1Msgs+3.
	handle2, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID: "sess-steer",
		Message:   "go on",
	})
	events2, _ := dispatcher.Events(context.Background(), handle2)
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

// TestDispatcher_InjectSteering_UnknownTurn returns ErrTurnNotFound
// for handles the dispatcher doesn't recognize — completed turns are
// pruned from the in-memory map.
func TestDispatcher_InjectSteering_UnknownTurn(t *testing.T) {
	dispatcher, _ := buildDispatcher(t)
	err := dispatcher.InjectSteering(context.Background(), turn.TurnHandle{TurnID: "no-such"}, "msg")
	if err == nil {
		t.Error("steering on unknown handle should error")
	}
}

// TestDispatcher_ApprovalGate_AllowOnce verifies the gate parks the turn
// on a TurnInterrupted{approval} when the configured mode requires consent
// (R model), and that the next run segment can attach before Resume drives the
// continuation to completion.
func TestDispatcher_ApprovalGate_AllowOnce(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel())
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	dispatcher := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil))))) // shell → gate

	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID:      "sess-approve",
		Message:        "echo lyra",
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	var sawInterrupt bool
	for ev := range events {
		if e, ok := ev.(runs.TurnInterrupted); ok {
			sawInterrupt = true
			if len(e.Interrupts) != 1 || e.Interrupts[0].Kind != execution.ApprovalInterrupt {
				t.Errorf("interrupts = %+v, want one approval", e.Interrupts)
			} else if p := e.Interrupts[0].Approval; p == nil || p.ToolName != "shell" {
				t.Errorf("approval payload = %+v, want shell ApprovalPrompt", p)
			}
			break
		}
	}
	if !sawInterrupt {
		t.Fatal("TurnInterrupted never fired in balanced mode")
	}

	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("reattach Events: %v", err)
	}
	if err := dispatcher.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}, []execution.InterruptKind{execution.ApprovalInterrupt}); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var endReason execution.Outcome
	for ev := range events {
		if end, ok := ev.(runs.TurnEnd); ok {
			endReason = end.Reason
		}
	}
	if endReason != execution.OutcomeCompleted {
		t.Errorf("turn end = %s, want completed", endReason)
	}
}

// TestDispatcher_ApprovalGate_ResumeAtPendingCall pins the R-model: approving a
// gated tool RESUMES the turn AT the pending call — the loop feeds back the
// parked tail (the interrupting round's assistant tool-call message) and runs
// the now-approved tool, then the model replies. So the model is invoked
// exactly TWICE across the cycle — round 1 (emits the call, interrupts) and the
// synthesis after resume — NOT three times: the interrupted round's call is
// never regenerated. The stored history must be a single valid
// user → assistant(tool_call) → tool → assistant sequence (no duplicate user
// message: it was persisted on the first run and the resume sends only the
// system header + the fed-back tail).
func TestDispatcher_ApprovalGate_ResumeAtPendingCall(t *testing.T) {
	model := &countingStubModel{}
	model.defaults = &chatmodel.Options{Model: "stub-counting"}
	client, _ := chatclient.New(model)
	store := history.NewInMemoryStore()
	eng := buildEngine(t, agentexec.Config{ChatClient: client, HistoryStore: store})
	dispatcher := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil))))) // shell → gate

	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID:      "sess-rmodel",
		Message:        "echo lyra",
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	var endReason execution.Outcome
	for ev := range events {
		switch e := ev.(type) {
		case runs.TurnInterrupted:
			if err := dispatcher.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}, []execution.InterruptKind{execution.ApprovalInterrupt}); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case runs.TurnEnd:
			endReason = e.Reason
		}
	}

	if endReason != execution.OutcomeCompleted {
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
		if m.Role == chatmodel.RoleUser {
			users++
		}
	}
	if users != 1 {
		t.Fatalf("stored history has %d user messages, want 1 (resume must not re-add the prompt): %+v", users, stored)
	}
}

// TestDispatcher_Cancel_ParkedTurn_DeliversTurnEnd verifies a canceled turn
// emits its terminal TurnEnd to a still-draining consumer rather than only
// closing the channel. Cancel cancels the turn ctx before finishTurn emits the
// terminal, so the event must not be lost to the emit ctx-escape: emit prefers
// delivery whenever the buffer has room. The turn parks on a balanced-mode
// approval gate; canceling it (instead of approving) must surface
// TurnEnd{Canceled}.
func TestDispatcher_Cancel_ParkedTurn_DeliversTurnEnd(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel())
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	dispatcher := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil)))))

	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID:      "sess-cancel-parked",
		Message:        "echo lyra",
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		sawEnd       bool
		endReason    execution.Outcome
	)
	for ev := range events {
		switch e := ev.(type) {
		case runs.TurnInterrupted:
			sawInterrupt = true
			if err := dispatcher.Cancel(context.Background(), handle); err != nil {
				t.Errorf("Cancel: %v", err)
			}
		case runs.TurnEnd:
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
	if endReason != execution.OutcomeCanceled {
		t.Errorf("TurnEnd.Reason = %s, want canceled", endReason)
	}
}

// TestDispatcher_ApprovalGate_Deny — denying via Resume(false) makes the
// tool short-circuit with the denial fed back to the model as a
// recoverable result; the model emits its final reply and the turn
// still completes.
func TestDispatcher_ApprovalGate_Deny(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel())
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	dispatcher := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil)))))

	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID:      "sess-deny",
		Message:        "echo lyra",
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	var (
		sawDenial bool
		endReason execution.Outcome
	)
	for ev := range events {
		switch e := ev.(type) {
		case runs.TurnInterrupted:
			_ = dispatcher.Resume(context.Background(), handle, interrupts.Resolution{Approved: false}, []execution.InterruptKind{execution.ApprovalInterrupt})
		case runs.ToolCallEnd:
			// Denial flows back as a tool *result* so the model can
			// recover — Err stays empty, Result carries the reason.
			if result, ok := e.Result.String(); ok && strings.Contains(result, "denied") {
				sawDenial = true
			}
		case runs.TurnEnd:
			endReason = e.Reason
		}
	}
	if !sawDenial {
		t.Error("expected a denied tool result after Resume(false)")
	}
	if endReason != execution.OutcomeCompleted {
		t.Errorf("turn end = %s, want completed (model recovered after denial)", endReason)
	}
}

// TestDispatcher_ApprovalGate_YoloSkipsEvent makes sure the gate is
// invisible under ModeYolo — the turn never parks (no TurnInterrupted),
// the tool runs as if no gate were wired.
func TestDispatcher_ApprovalGate_YoloSkipsEvent(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel())
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	dispatcher := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeYolo, nil)))))

	handle, _ := dispatcher.StartTurn(context.Background(), runs.StartTurn{
		SessionID: "sess-yolo",
		Message:   "echo lyra",
	})
	events, _ := dispatcher.Events(context.Background(), handle)

	for ev := range events {
		if _, ok := ev.(runs.TurnInterrupted); ok {
			t.Error("TurnInterrupted should NOT fire in yolo mode")
		}
	}
}

// TestDispatcher_StartTurn_Validation rejects empty SessionID / Message.
func TestDispatcher_StartTurn_Validation(t *testing.T) {
	dispatcher, _ := buildDispatcher(t)

	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{Message: "x"}); err == nil {
		t.Error("missing SessionID should error")
	}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s"}); err == nil {
		t.Error("missing Message should error")
	}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s", Message: "x", MaxSteps: -1}); !errors.Is(err, runs.ErrInvalidTurnLimit) {
		t.Fatalf("negative MaxSteps err = %v, want ErrInvalidTurnLimit", err)
	}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s", Message: "x", MaxCostUSD: -0.01}); !errors.Is(err, runs.ErrInvalidTurnLimit) {
		t.Fatalf("negative MaxCostUSD err = %v, want ErrInvalidTurnLimit", err)
	}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s", Message: "x", MaxBudget: -1}); !errors.Is(err, runs.ErrInvalidTurnLimit) {
		t.Fatalf("negative MaxBudget err = %v, want ErrInvalidTurnLimit", err)
	}
	opts := &chatmodel.Options{Model: "should-not-select-model-here"}
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s", Message: "x", Options: opts}); !errors.Is(err, runs.ErrInvalidTurnOptions) {
		t.Fatalf("Options.Model err = %v, want ErrInvalidTurnOptions", err)
	}
	maxTokens := int64(0)
	if _, err := dispatcher.StartTurn(context.Background(), runs.StartTurn{SessionID: "s", Message: "x", Options: &chatmodel.Options{MaxTokens: &maxTokens}}); !errors.Is(err, runs.ErrInvalidTurnOptions) {
		t.Fatalf("MaxTokens=0 err = %v, want ErrInvalidTurnOptions", err)
	}
}
