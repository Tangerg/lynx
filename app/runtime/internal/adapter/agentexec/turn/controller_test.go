package turn_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/chathistory/inmemory"
	chatmodel "github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/conversations"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// TestController_StartTurn_EmitsExpectedEvents drives a full turn
// against a stub LLM that asks for `shell` (echo lyra). The controller
// must emit the canonical sequence:
//
//	UsageReported → ToolCallStarted → ToolCallFinished → MessageDelta → UsageReported → TurnEnd
//
// and the sequence must end cleanly.
func TestController_StartTurn_EmitsExpectedEvents(t *testing.T) {
	controller, _ := buildController(t)

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-1",
		Message:   "say lyra via shell",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if _, err := controller.Events(context.Background(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("concurrent Events = %v, want ErrTurnNotFound", err)
	}

	got := drainEvents(events)

	wantOrder := []string{
		"UsageReported",
		"ToolCallStarted",
		"ToolCallFinished",
		"MessageDelta",
		"UsageReported",
		"TurnEnd",
	}
	if names := eventNames(got); !sliceEqual(names, wantOrder) {
		t.Fatalf("event order mismatch:\n  got  %v\n  want %v", names, wantOrder)
	}

	// Spot-check each event's content.
	for _, ev := range got {
		switch e := ev.(type) {
		case runs.ToolCallStarted:
			if e.ToolName != "shell" {
				t.Errorf("ToolCallStarted.ToolName = %q, want shell", e.ToolName)
			}
			if !strings.Contains(e.Arguments, "echo lyra") {
				t.Errorf("ToolCallStarted.Arguments missing command: %q", e.Arguments)
			}
			if e.Activity != "Print lyra" {
				t.Errorf("ToolCallStarted.Activity = %q, want shell description", e.Activity)
			}
		case runs.ToolCallFinished:
			if e.Problem != nil {
				t.Errorf("ToolCallFinished.Problem = %+v, want nil", e.Problem)
			}
			result, ok := e.Result.Any().(map[string]any)
			if !ok {
				t.Fatalf("ToolCallFinished.Result = %T, want JSON object", e.Result)
			}
			output, ok := result["output"].(string)
			if !ok || !strings.Contains(output, "lyra") {
				t.Errorf("ToolCallFinished.Result missing 'lyra': %#v", e.Result)
			}
		case runs.MessageDelta:
			if !strings.Contains(e.Text, "lyra") {
				t.Errorf("MessageDelta.Text missing 'lyra': %q", e.Text)
			}
		case runs.SegmentEnded:
			if e.Reason != run.OutcomeCompleted {
				t.Errorf("TurnEnd.Reason = %s, want completed", e.Reason)
			}
		}
	}

	joinTurnCleanup(t, controller, handle)
	if err := controller.Cancel(t.Context(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Errorf("Cancel after cleanup = %v, want ErrTurnNotFound", err)
	}
}

func TestControllerCloseCancelsLiveTurnsAndRejectsAdmission(t *testing.T) {
	controller := mustTurn(turn.New(turnDeps(&slowStubEngine{})))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-close",
		Message:   "wait",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	shutdownController(t, controller)
	var endReason run.Outcome
	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok {
			endReason = end.Reason
		}
	}
	if endReason != run.OutcomeCanceled {
		t.Fatalf("TurnEnd reason = %q, want canceled", endReason)
	}
	if _, err := controller.Events(context.Background(), handle); !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("Events after Close = %v, want ErrTurnNotFound", err)
	}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "new", Message: "no"}); !errors.Is(err, turn.ErrClosed) {
		t.Fatalf("StartTurn after Close = %v, want ErrClosed", err)
	}
	shutdownController(t, controller)
}

func TestNewRequiresApprovalGate(t *testing.T) {
	_, err := turn.New(turn.Dependencies{Engine: &stubEngine{}})
	if err == nil {
		t.Fatal("New accepted a controller without an approval gate")
	}
}

// TestController_InjectSteering_PreservesStructuredContent verifies both legal
// scheduling outcomes share one interpretation: the next live model round may
// consume the steer, or terminal flush persists it for the next turn. Either
// way the model sees the ordered text+image user message.
func TestController_InjectSteering_PreservesStructuredContent(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chatclient.New(stub, chatclient.Config{})
	store := inmemory.New()
	eng := buildEngine(t, agentexec.Config{ChatClient: client, HistoryStore: store})
	controller := mustTurn(turn.New(turnDeps(eng, func(deps *turn.Dependencies) {
		deps.Steering = conversations.NewMessages(store)
	})))

	// Turn 1.
	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-steer",
		Message:   "hi",
	})
	events, _ := controller.Events(context.Background(), handle)

	// Inject steering before consuming events so the controller has
	// time to land it on the turn state. Drain the channel before
	// starting turn 2 — the steering flushes after the turn returns.
	if err := controller.InjectSteering(context.Background(), handle, []transcript.ContentBlock{
		{Kind: transcript.TextContent, Text: "also keep responses short"},
		{Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte("image")},
	}); err != nil {
		t.Fatalf("InjectSteering: %v", err)
	}
	turn1Events := drainEvents(events)
	var steerEvents []runs.SteerMessage
	for _, event := range turn1Events {
		if steer, ok := event.(runs.SteerMessage); ok {
			steerEvents = append(steerEvents, steer)
		}
	}
	if len(steerEvents) != 1 ||
		len(steerEvents[0].Content) != 2 ||
		steerEvents[0].Content[0].Text != "also keep responses short" ||
		steerEvents[0].Content[1].Kind != transcript.ImageContent {
		t.Fatalf("steering events = %+v, want one canonical text+image event", steerEvents)
	}

	// Start a second turn so a steer that missed the live source is observable
	// through the terminal history fallback.
	handle2, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-steer",
		Message:   "go on",
	})
	events2, _ := controller.Events(context.Background(), handle2)
	for range events2 {
	}

	if len(stub.seenLengths) < 2 {
		t.Fatalf("stub Call count = %d, want >= 2", len(stub.seenLengths))
	}
	var foundStructuredSteer bool
	for _, requestMessages := range stub.seenMessages {
		for _, message := range requestMessages {
			if message.Role != chatmodel.RoleUser || message.Text() != "also keep responses short" {
				continue
			}
			if len(message.Parts) == 2 &&
				message.Parts[0].Kind == chatmodel.PartText &&
				message.Parts[1].Kind == chatmodel.PartMedia &&
				message.Parts[1].Media != nil &&
				message.Parts[1].Media.MIME == "image/png" {
				foundStructuredSteer = true
				break
			}
		}
	}
	if !foundStructuredSteer {
		t.Fatalf("model requests = %+v, want ordered text+image steering input", stub.seenMessages)
	}
}

// TestController_InjectSteering_UnknownTurn returns ErrTurnNotFound
// for handles the controller doesn't recognize — completed turns are
// pruned from the in-memory map.
func TestController_InjectSteering_UnknownTurn(t *testing.T) {
	controller, _ := buildController(t)
	err := controller.InjectSteering(context.Background(), turn.Handle{TurnID: "no-such"}, []transcript.ContentBlock{{
		Kind: transcript.TextContent, Text: "msg",
	}})
	if err == nil {
		t.Error("steering on unknown handle should error")
	}

	err = controller.InjectSteering(context.Background(), turn.Handle{TurnID: "no-such"}, nil)
	if !errors.Is(err, runs.ErrInputRequired) {
		t.Fatalf("empty steering error = %v, want ErrInputRequired before turn lookup", err)
	}
}

// TestController_ApprovalGate_AllowOnce verifies the gate parks the turn
// on a TreeInterrupted{approval} when the configured mode requires consent
// (R model), and that the next run segment can attach before Resume drives the
// continuation to completion.
func TestController_ApprovalGate_AllowOnce(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel(), chatclient.Config{})
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	controller := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil))))) // shell → gate

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "sess-approve",
		Message:        "echo lyra",
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	})
	events, _ := controller.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		barrier      runs.TreeInterrupted
	)
	for ev := range events {
		if e, ok := ev.Payload.(runs.TreeInterrupted); ok {
			sawInterrupt = true
			barrier = e
			if len(e.Suspensions) != 1 ||
				e.Suspensions[0].Interrupt.Kind != interrupt.Approval {
				t.Errorf("suspensions = %+v, want one approval", e.Suspensions)
			} else if p := e.Suspensions[0].Interrupt.Approval; p == nil || p.ToolName != "shell" {
				t.Errorf("approval payload = %+v, want shell ApprovalPrompt", p)
			}
			break
		}
	}
	if !sawInterrupt {
		t.Fatal("TreeInterrupted never fired in balanced mode")
	}

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("reattach Events: %v", err)
	}
	if err := controller.Resume(
		context.Background(),
		handle,
		answersForBarrier(barrier, interrupt.Resolution{Approved: true}),
		[]interrupt.Kind{interrupt.Approval},
	); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var endReason run.Outcome
	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok {
			endReason = end.Reason
		}
	}
	if endReason != run.OutcomeCompleted {
		t.Errorf("turn end = %s, want completed", endReason)
	}
}

// TestController_ApprovalGate_ResumeAtPendingCall pins the R-model: approving a
// gated tool RESUMES the turn AT the pending call — the loop feeds back the
// parked tail (the interrupting round's assistant tool-call message) and runs
// the now-approved tool, then the model replies. So the model is invoked
// exactly TWICE across the cycle — round 1 (emits the call, interrupts) and the
// synthesis after resume — NOT three times: the interrupted round's call is
// never regenerated. The stored history must be a single valid
// user → assistant(tool_call) → tool → assistant sequence (no duplicate user
// message: it was persisted on the first run and the resume sends only the
// system header + the fed-back tail).
func TestController_ApprovalGate_ResumeAtPendingCall(t *testing.T) {
	model := &countingStubModel{}
	model.defaults = &chatmodel.Options{Model: "stub-counting"}
	client, _ := chatclient.New(model, chatclient.Config{})
	store := inmemory.New()
	eng := buildEngine(t, agentexec.Config{ChatClient: client, HistoryStore: store})
	controller := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil))))) // shell → gate

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "sess-rmodel",
		Message:        "echo lyra",
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	})
	events, _ := controller.Events(context.Background(), handle)

	var endReason run.Outcome
	for ev := range events {
		switch e := ev.Payload.(type) {
		case runs.TreeInterrupted:
			if err := controller.Resume(
				context.Background(),
				handle,
				answersForBarrier(e, interrupt.Resolution{Approved: true}),
				[]interrupt.Kind{interrupt.Approval},
			); err != nil {
				t.Errorf("Resume: %v", err)
			}
		case runs.SegmentEnded:
			endReason = e.Reason
		}
	}

	if endReason != run.OutcomeCompleted {
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

// TestController_Cancel_ParkedTurn_DeliversTurnEnd verifies a canceled turn
// emits its terminal TurnEnd to a still-draining consumer rather than only
// closing the channel. Cancel cancels the turn ctx before finishTurn emits the
// terminal, so the event must not be lost to the emit ctx-escape: emit prefers
// delivery whenever the buffer has room. The turn parks on a balanced-mode
// approval gate; canceling it (instead of approving) must surface
// TurnEnd{Canceled}.
func TestController_Cancel_ParkedTurn_DeliversTurnEnd(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel(), chatclient.Config{})
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	controller := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil)))))

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "sess-cancel-parked",
		Message:        "echo lyra",
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	})
	events, _ := controller.Events(context.Background(), handle)

	var (
		sawInterrupt bool
		sawEnd       bool
		endReason    run.Outcome
	)
	for ev := range events {
		switch e := ev.Payload.(type) {
		case runs.TreeInterrupted:
			sawInterrupt = true
			if err := controller.Cancel(context.Background(), handle); err != nil {
				t.Errorf("Cancel: %v", err)
			}
		case runs.SegmentEnded:
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
	if endReason != run.OutcomeCanceled {
		t.Errorf("TurnEnd.Reason = %s, want canceled", endReason)
	}
}

// TestController_ApprovalGate_Deny — denying via Resume(false) makes the
// tool short-circuit with the denial fed back to the model as a
// recoverable result; the model emits its final reply and the turn
// still completes.
func TestController_ApprovalGate_Deny(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel(), chatclient.Config{})
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	controller := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeBalanced, nil)))))

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "sess-deny",
		Message:        "echo lyra",
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	})
	events, _ := controller.Events(context.Background(), handle)

	var (
		sawDenial bool
		endReason run.Outcome
	)
	for ev := range events {
		switch e := ev.Payload.(type) {
		case runs.TreeInterrupted:
			_ = controller.Resume(
				context.Background(),
				handle,
				answersForBarrier(e, interrupt.Resolution{Approved: false}),
				[]interrupt.Kind{interrupt.Approval},
			)
		case runs.ToolCallFinished:
			// Denial flows back as a tool *result* so the model can
			// recover — Err stays empty, Result carries the reason.
			if result, ok := e.Result.String(); ok && strings.Contains(result, "denied") {
				sawDenial = true
			}
		case runs.SegmentEnded:
			endReason = e.Reason
		}
	}
	if !sawDenial {
		t.Error("expected a denied tool result after Resume(false)")
	}
	if endReason != run.OutcomeCompleted {
		t.Errorf("turn end = %s, want completed (model recovered after denial)", endReason)
	}
}

// TestController_ApprovalGate_YoloSkipsEvent makes sure the gate is
// invisible under ModeYolo — the turn never parks (no TreeInterrupted),
// the tool runs as if no gate were wired.
func TestController_ApprovalGate_YoloSkipsEvent(t *testing.T) {
	client, _ := chatclient.New(newStubChatModel(), chatclient.Config{})
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	controller := mustTurn(turn.New(turnDeps(eng, withApproval(mustApprovalPolicy(t, approval.ModeYolo, nil)))))

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-yolo",
		Message:   "echo lyra",
	})
	events, _ := controller.Events(context.Background(), handle)

	for ev := range events {
		if _, ok := ev.Payload.(runs.TreeInterrupted); ok {
			t.Error("TreeInterrupted should NOT fire in yolo mode")
		}
	}
}

// TestController_StartTurn_Validation rejects empty SessionID / Message.
func TestController_StartTurn_Validation(t *testing.T) {
	controller, _ := buildController(t)

	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{Message: "x"}); err == nil {
		t.Error("missing SessionID should error")
	}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s"}); err == nil {
		t.Error("missing Message should error")
	}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "x", Limits: run.RunLimits{MaxSteps: -1}}); !errors.Is(err, runs.ErrInvalidRunLimit) {
		t.Fatalf("negative MaxSteps err = %v, want ErrInvalidTurnLimit", err)
	}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "x", Limits: run.RunLimits{MaxBudgetUSD: -0.01}}); !errors.Is(err, runs.ErrInvalidRunLimit) {
		t.Fatalf("negative MaxCostUSD err = %v, want ErrInvalidTurnLimit", err)
	}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "x", Limits: run.RunLimits{MaxTotalTokens: -1}}); !errors.Is(err, runs.ErrInvalidRunLimit) {
		t.Fatalf("negative MaxTotalTokens err = %v, want ErrInvalidTurnLimit", err)
	}
	opts := &chatmodel.Options{Model: "should-not-select-model-here"}
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "x", Options: opts}); !errors.Is(err, runs.ErrInvalidRunOptions) {
		t.Fatalf("Options.Model err = %v, want ErrInvalidTurnOptions", err)
	}
	maxTokens := int64(0)
	if _, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "x", Options: &chatmodel.Options{MaxTokens: &maxTokens}}); !errors.Is(err, runs.ErrInvalidRunOptions) {
		t.Fatalf("MaxTokens=0 err = %v, want ErrInvalidTurnOptions", err)
	}
}
