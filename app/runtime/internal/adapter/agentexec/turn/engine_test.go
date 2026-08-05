package turn_test

import (
	"context"
	"errors"
	"iter"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// TestStubEngineDrivesTurn — confirms the turn controller runs a full
// turn against a stub engine, no real engine involved. If turn
// ever regrows a hard *agentexec.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "sess-1",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := controller.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	var sawDelta, sawEnd bool
	for ev := range events {
		switch ev.Payload.(type) {
		case runs.MessageDelta:
			sawDelta = true
		case runs.SegmentEnded:
			sawEnd = true
		}
	}
	if !sawEnd {
		t.Fatalf("timed out without TurnEnd; sawDelta=%v sawEnd=%v", sawDelta, sawEnd)
	}

	if !sawDelta {
		t.Errorf("expected at least one MessageDelta event")
	}
	if got := stub.runTurnCalls.Load(); got != 1 {
		t.Errorf("StartTurn called %d times, want 1", got)
	}
}

func TestStartTurnPreservesHookResolutionFailure(t *testing.T) {
	wantErr := errors.New("hook trust unavailable")
	stub := &stubEngine{runReply: "must not run"}
	controller := mustTurn(turn.New(turnDeps(stub, func(deps *turn.Dependencies) {
		deps.Hooks = staticHookResolver{err: wantErr}
	})))
	t.Cleanup(func() { shutdownController(t, controller) })

	if _, err := controller.StartTurn(t.Context(), runs.StartExecution{
		SessionID: "sess-hook-error",
		Message:   "hi",
		Cwd:       t.TempDir(),
	}); !errors.Is(err, wantErr) {
		t.Fatalf("StartTurn error = %v, want %v", err, wantErr)
	}
	if got := stub.runTurnCalls.Load(); got != 0 {
		t.Fatalf("engine StartTurn calls = %d, want 0", got)
	}
}

// TestPromptHookInjectedContextReachesTurn guards the prepare/activate seam: a
// UserPromptSubmit hook that injects context must have it in the prompt the
// engine actually runs. The request is snapshotted for Activate AFTER the prompt
// hooks rewrite the message; capturing it before would silently drop the
// injection.
func TestPromptHookInjectedContextReachesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	bound := hooks.NewBound(
		[]hooks.Hook{{Event: hooks.UserPromptSubmit, Inject: "remember: use tabs", Source: "test"}},
		hooks.NewRunner(nil, nil), // declarative inject needs no command runner
	)
	controller := mustTurn(turn.New(turnDeps(stub, func(deps *turn.Dependencies) {
		deps.Hooks = staticHookResolver{bound: bound}
	})))
	t.Cleanup(func() { shutdownController(t, controller) })

	handle, err := controller.StartTurn(t.Context(), runs.StartExecution{
		SessionID: "s", Message: "do the thing", Cwd: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)
	for range events { //nolint:revive // drain to terminal
	}

	got := stub.message()
	if !strings.Contains(got, "remember: use tabs") || !strings.Contains(got, "do the thing") {
		t.Fatalf("engine prompt = %q, want the injected context AND the original message", got)
	}
}

// TestController_DiscardsProcessOnTerminal verifies terminal teardown removes
// the in-memory process tree and any previously parked checkpoint. Terminal
// event delivery is deliberately independent from cleanup, so the test
// explicitly joins that cleanup before inspecting its postcondition.
func TestController_DiscardsProcessOnTerminal(t *testing.T) {
	stub := &stubEngine{runReply: "done"}
	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)
	for range events { //nolint:revive // drain to terminal (channel closes at terminalization)
	}
	joinTurnCleanup(t, controller, handle)
	process := stub.lastProcess.Load()
	if process == nil {
		t.Fatal("stub engine never produced a process")
	}
	if !process.discarded.Load() {
		t.Error("process not discarded at terminal teardown")
	}
}

func TestControllerFailsClosedWhenWaitingCheckpointCommitFails(t *testing.T) {
	wantErr := errors.New("checkpoint commit failed")
	stub := &stubEngine{
		completionStatus: core.StatusWaiting,
		completionErr:    wantErr,
	}
	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.StartTurn(t.Context(), runs.StartExecution{
		SessionID: "s",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	var interrupted bool
	var terminal *runs.SegmentEnded
	for event := range events {
		switch event := event.Payload.(type) {
		case runs.TreeInterrupted:
			interrupted = true
		case runs.SegmentEnded:
			value := event
			terminal = &value
		}
	}
	joinTurnCleanup(t, controller, handle)
	if interrupted {
		t.Fatal("turn exposed an interrupt without a durable checkpoint")
	}
	if terminal == nil || terminal.Reason != execution.OutcomeError ||
		terminal.Problem == nil || terminal.Problem.Kind != transcript.InternalProblem {
		t.Fatalf("terminal = %+v, want internal error", terminal)
	}
	process := stub.lastProcess.Load()
	if process == nil || !process.discarded.Load() {
		t.Fatal("failed parked process was not discarded")
	}
}

// TestStubEngineBudgetStop — a turn whose process reports
// StopReasonBudget ends with Reason=execution.OutcomeMaxBudget, not a plain
// completion, so clients can tell "stopped at the ceiling" apart from
// "model finished".
func TestStubEngineBudgetStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopReason: agent.InteractionStopBudget}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "go",
		Limits:    execution.RunLimits{MaxTotalTokens: 1},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)

	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok {
			if end.Reason != execution.OutcomeMaxBudget {
				t.Fatalf("TurnEnd reason = %v, want budget_exceeded", end.Reason)
			}
			return
		}
	}
	t.Fatal("no TurnEnd within 2s")
}

// TestStubEngineStepStop — the same treatment for a step stop. This covers the
// turn's own tool-round guardrail as well as a caller's MaxSteps, because both
// arrive as InteractionStopModelCalls: reaching either is a real outcome carrying the
// partial reply, never an error with a client-facing problem attached.
func TestStubEngineStepStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopReason: agent.InteractionStopModelCalls}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "go",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)

	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok {
			if end.Reason != execution.OutcomeMaxSteps {
				t.Fatalf("TurnEnd reason = %v, want max steps", end.Reason)
			}
			if end.Problem != nil {
				t.Fatalf("TurnEnd problem = %+v, want none for a bounded stop", end.Problem)
			}
			return
		}
	}
	t.Fatal("no TurnEnd within 2s")
}

func TestStubEngineInvalidStopReasonBecomesEngineError(t *testing.T) {
	stub := &stubEngine{
		runReply:   "invalid",
		stopReason: agent.InteractionStopReason("budget+steps"),
	}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "go",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := controller.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	var sawEnd bool
	for event := range events {
		switch value := event.Payload.(type) {
		case runs.SegmentEnded:
			sawEnd = value.Reason == execution.OutcomeError && value.Problem != nil && value.Problem.Kind == transcript.InternalProblem
		}
	}
	if !sawEnd {
		t.Fatal("invalid stop reason did not produce an error TurnEnd with an internal problem")
	}
}

// TestStubEngineCancelsCleanly — confirms Cancel propagates to the
// turn without needing a real engine.
func TestStubEngineCancelsCleanly(t *testing.T) {
	stub := &slowStubEngine{}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, _ := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "m",
	})
	if err := controller.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := controller.Events(ctx, handle)
	if err != nil {
		// Cancel raced ahead and tore the turn down (parked-turn teardown, or
		// the drive goroutine finishing) before we subscribed — Events then
		// returns ErrTurnNotFound + a nil iterator. The turn is gone, which is
		// exactly the clean cancel this test asserts, so don't range a nil.
		return
	}
	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok && end.Reason == execution.OutcomeCanceled {
			return
		}
	}
	// Iterator drained: either a TurnEnd(Canceled) returned above, or the
	// channel closed on turn done. Reaching here only on the 2s ctx timeout.
	if ctx.Err() != nil {
		t.Fatalf("turn did not cancel within 2s")
	}
}

// TestRehydrateResumesRestoredTurn covers the cross-restart two-phase path: a
// rehydrated turn first exposes its parked process, then Resume delivers the
// decision and streams the continuation on the already-observable handle.
func TestRehydrateResumesRestoredTurn(t *testing.T) {
	stub := &stubEngine{runReply: "continuation reply"}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, err := controller.Rehydrate(context.Background(), runs.RehydrateExecution{
		SessionID:  "sess-restored",
		ExecutorID: "turn-original",
		ProcessID:  "process-42",
		RootRunID:  "run-root",
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if handle.TurnID != "turn-original" {
		t.Fatalf("Rehydrate turn id = %q, want persisted turn-original", handle.TurnID)
	}
	if got := stub.restoreCalls.Load(); got != 1 {
		t.Fatalf("RestoreTurn calls = %d, want 1", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := controller.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := controller.Resume(
		ctx,
		handle,
		nil,
		nil,
	); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var sawDelta, sawEnd bool
	for ev := range events {
		switch e := ev.Payload.(type) {
		case runs.MessageDelta:
			sawDelta = true
		case runs.SegmentEnded:
			sawEnd = true
			if e.Reason != execution.OutcomeCompleted {
				t.Errorf("TurnEnd reason = %s, want completed", e.Reason)
			}
		}
	}
	if !sawDelta {
		t.Error("rehydrated continuation produced no MessageDelta")
	}
	if !sawEnd {
		t.Error("rehydrated turn never reached TurnEnd")
	}
}

// TestRehydrate_ResumeError_ReturnsError proves a synchronous resume failure is
// still observable: Rehydrate returns the parked handle, Events attaches, then
// Resume emits an error TurnEnd before returning its error.
func TestRehydrate_ResumeError_ReturnsError(t *testing.T) {
	stub := &stubEngine{runReply: "x", restoreResumeErr: errors.New("resume boom")}
	controller := mustTurn(turn.New(turnDeps(stub)))

	handle, err := controller.Rehydrate(context.Background(), runs.RehydrateExecution{
		SessionID:  "sess-restored",
		ExecutorID: "turn-original",
		ProcessID:  "process-99",
		RootRunID:  "run-root",
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := controller.Resume(
		context.Background(),
		handle,
		nil,
		nil,
	); err == nil {
		t.Fatal("Resume returned nil error despite the restored process failure")
	}
	var sawEnd bool
	for ev := range events {
		if end, ok := ev.Payload.(runs.SegmentEnded); ok {
			sawEnd = end.Reason == execution.OutcomeError && end.Problem != nil
		}
	}
	if !sawEnd {
		t.Fatal("terminal stream did not contain an error TurnEnd")
	}
	if _, evErr := controller.Events(context.Background(), handle); evErr == nil {
		t.Error("Events resolved a turn that should have been torn down")
	}
}

func TestRehydrateCanceledResumeAdmissionRemainsParked(t *testing.T) {
	stub := &stubEngine{runReply: "continuation reply", restoreResumeErr: context.Canceled}
	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
		SessionID:  "sess-restored",
		ExecutorID: "turn-original",
		ProcessID:  "process-99",
		RootRunID:  "run-root",
	})
	if err != nil {
		t.Fatal(err)
	}
	events, err := controller.Events(t.Context(), handle)
	if err != nil {
		t.Fatal(err)
	}

	if err := controller.Resume(t.Context(), handle, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("first Resume = %v, want context cancellation", err)
	}
	process := stub.lastProcess.Load()
	if process == nil {
		t.Fatal("restored process was not retained")
	}
	process.resumeErr = nil
	if err := controller.Resume(t.Context(), handle, nil, nil); err != nil {
		t.Fatalf("retry Resume: %v", err)
	}
	for range events {
	}
}

// TestStartTurn_ResolvesPerRunClient verifies a turn carrying a Model passes
// the resolver's client through to the engine's TurnRequest.ChatClient —
// the turn-controller half of per-run model selection.
func TestStartTurn_ResolvesPerRunClient(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	sentinel, _ := chatclient.New(newCapturingModel(), chatclient.Config{})
	resolver := &fakeResolver{client: sentinel}

	controller := mustTurn(turn.New(turnDeps(stub, withClientResolver(resolver))))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "s",
		Message:        "hi",
		ModelSelection: testModelSelection(t, "some-provider", "some-model"),
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)
	for range events { // drain to TurnEnd
	}

	if resolver.gotProvider != "some-provider" || resolver.gotModel != "some-model" {
		t.Errorf("resolver asked for (%q,%q), want (some-provider, some-model)", resolver.gotProvider, resolver.gotModel)
	}
	stub.mu.Lock()
	got := stub.lastClient
	stub.mu.Unlock()
	if got != sentinel {
		t.Errorf("engine received ChatClient %p, want the resolver's client %p", got, sentinel)
	}
}

func TestExplicitModelSelectionRequiresResolverBeforeAdmission(t *testing.T) {
	controller := mustTurn(turn.New(turnDeps(&stubEngine{})))
	selection := testModelSelection(t, "openai", "gpt-test")
	if _, err := controller.PrepareTurn(t.Context(), runs.StartExecution{
		SessionID: "session", Message: "hello", ModelSelection: selection,
	}); err == nil || !strings.Contains(err.Error(), "requires a client resolver") {
		t.Fatalf("PrepareTurn error = %v, want missing resolver", err)
	}
	if _, err := controller.Rehydrate(t.Context(), runs.RehydrateExecution{
		SessionID: "session", ProcessID: "process", RootRunID: "run-root", ModelSelection: selection,
	}); err == nil || !strings.Contains(err.Error(), "requires a client resolver") {
		t.Fatalf("Rehydrate error = %v, want missing resolver", err)
	}
}

// TestStartTurn_PassesCwd verifies the session's working directory flows
// from runs.StartTurn.Cwd through to the engine's TurnRequest.Cwd —
// the turn-controller half of per-session tool working directories.
func TestStartTurn_PassesCwd(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}

	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "hi",
		Cwd:       "/work/project-a",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)
	for range events { // drain to TurnEnd
	}

	stub.mu.Lock()
	got := stub.lastCwd
	stub.mu.Unlock()
	if got != "/work/project-a" {
		t.Errorf("engine received Cwd %q, want %q", got, "/work/project-a")
	}
}

func TestStartTurn_PassesOptions(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	temp := 0.7

	controller := mustTurn(turn.New(turnDeps(stub)))
	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "s",
		Message:   "hi",
		Options:   &corechat.Options{Temperature: &temp},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := controller.Events(ctx, handle)
	for range events { // drain to TurnEnd
	}

	stub.mu.Lock()
	got := stub.lastOptions
	stub.mu.Unlock()
	if got == nil || got.Temperature == nil || *got.Temperature != 0.7 {
		t.Fatalf("engine options = %+v, want temperature 0.7", got)
	}
}

func TestStartTurnSnapshotsMutableRequestValues(t *testing.T) {
	engine := newDelayedCaptureEngine()
	controller := mustTurn(turn.New(turnDeps(engine)))

	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("media.NewBytes: %v", err)
	}
	temperature := 0.7
	frequencyPenalty := 0.4
	topK := int64(4)
	options := &corechat.Options{
		Temperature:      &temperature,
		FrequencyPenalty: &frequencyPenalty,
		TopK:             &topK,
		Stop:             []string{"done"},
	}
	images := []*media.Media{image}
	interruptKinds := []execution.InterruptKind{execution.ApprovalInterrupt}

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID:      "session",
		Message:        "hello",
		Media:          images,
		Options:        options,
		InterruptKinds: interruptKinds,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	<-engine.entered

	*options.Temperature = 1.4
	*options.FrequencyPenalty = 1.5
	*options.TopK = 8
	options.Stop[0] = "changed"
	image.Source.Bytes[0] = 9
	images[0] = nil
	interruptKinds[0] = execution.QuestionInterrupt
	close(engine.release)

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	for range events {
	}

	captured := <-engine.captured
	if captured.temperature != 0.7 {
		t.Errorf("temperature = %v, want 0.7", captured.temperature)
	}
	if captured.frequencyPenalty != 0.4 {
		t.Errorf("frequency penalty = %v, want 0.4", captured.frequencyPenalty)
	}
	if captured.topK != 4 {
		t.Errorf("top k = %d, want 4", captured.topK)
	}
	if captured.stop != "done" {
		t.Errorf("stop = %q, want done", captured.stop)
	}
	if captured.mediaByte != 1 {
		t.Errorf("media byte = %d, want 1", captured.mediaByte)
	}
}

func TestStartTurnProcessCreationFailureRemainsDrainableAfterTerminal(t *testing.T) {
	startErr := errors.New("create process failed")
	engine := &immediateStartFailureEngine{err: startErr}
	controller := mustTurn(turn.New(turnDeps(engine)))

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "session",
		Message:   "hello",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	waitForTurnRemoval(t, controller, handle)

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events after terminal create failure: %v", err)
	}
	assertCreateFailureEvents(t, events, startErr)
}

func TestStartTurnCancelRacingProcessCreationFailureTerminatesAsCanceled(t *testing.T) {
	startErr := errors.New("create process failed")
	engine := newBlockedStartFailureEngine(startErr)
	controller := mustTurn(turn.New(turnDeps(engine)))

	handle, err := controller.StartTurn(context.Background(), runs.StartExecution{
		SessionID: "session",
		Message:   "hello",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	<-engine.entered
	if err := controller.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	close(engine.release)
	waitForTurnRemoval(t, controller, handle)

	events, err := controller.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events after cancel/create failure race: %v", err)
	}
	var terminals int
	for event := range events {
		if end, ok := event.Payload.(runs.SegmentEnded); ok {
			terminals++
			if end.Reason != execution.OutcomeCanceled || end.Problem != nil {
				t.Errorf("TurnEnd = %+v, want cancellation without a problem", end)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal events = %d, want 1", terminals)
	}
}

func waitForTurnRemoval(t *testing.T, controller turnDriver, handle turn.Handle) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := controller.ProcessID(context.Background(), handle)
		if errors.Is(err, turn.ErrTurnNotFound) {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("turn was not removed after process creation failure")
}

func assertCreateFailureEvents(t *testing.T, events iter.Seq[runs.ExecutorEvent], startErr error) {
	t.Helper()

	var terminals int
	for event := range events {
		switch value := event.Payload.(type) {
		case runs.SegmentEnded:
			terminals++
			if value.Reason != execution.OutcomeError || value.Problem == nil || value.Problem.Kind != transcript.InternalProblem {
				t.Errorf("TurnEnd = %+v, want error with an internal problem", value)
			}
			if value.Problem != nil && strings.Contains(value.Problem.Detail, startErr.Error()) {
				t.Errorf("TurnEnd.Problem leaked startup error %q", startErr)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal events = %d, want 1", terminals)
	}
}
