package turn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"
)

// TestStubEngineDrivesTurn — confirms the turn dispatcher runs a full
// turn against a stub engine, no real engine involved. If turn
// ever regrows a hard *agentexec.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-1",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := dispatcher.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	var sawDelta, sawEnd bool
	for ev := range events {
		switch ev.(type) {
		case turn.MessageDelta:
			sawDelta = true
		case turn.TurnEnd:
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

// TestDispatcher_DiscardsProcessOnTerminal verifies the turn discards its backing
// process at terminal teardown (endTurn → TurnProcess.Discard) — the seam that
// deletes the auto-snapshot. Without it every run leaks one process_snapshot
// row. The events channel closes only after endTurn runs, so reading the flag
// after the drain loop is race-free.
func TestDispatcher_DiscardsProcessOnTerminal(t *testing.T) {
	stub := &stubEngine{runReply: "done"}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)
	for range events { //nolint:revive // drain to terminal (channel closes after endTurn)
	}
	process := stub.lastProcess.Load()
	if process == nil {
		t.Fatal("stub engine never produced a process")
	}
	if !process.discarded.Load() {
		t.Error("process not discarded at terminal teardown — snapshot would leak")
	}
}

// TestStubEngineBudgetStop — a turn whose process reports
// StoppedOnBudget ends with Reason=execution.OutcomeMaxBudget, not a plain
// completion, so clients can tell "stopped at the ceiling" apart from
// "model finished".
func TestStubEngineBudgetStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopOnBudget: true}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))

	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "go",
		MaxBudget: 1,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)

	for ev := range events {
		if end, ok := ev.(turn.TurnEnd); ok {
			if end.Reason != execution.OutcomeMaxBudget {
				t.Fatalf("TurnEnd reason = %v, want budget_exceeded", end.Reason)
			}
			return
		}
	}
	t.Fatal("no TurnEnd within 2s")
}

// TestStubEngineCancelsCleanly — confirms Cancel propagates to the
// turn without needing a real engine.
func TestStubEngineCancelsCleanly(t *testing.T) {
	stub := &slowStubEngine{}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))

	handle, _ := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "m",
	})
	if err := dispatcher.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := dispatcher.Events(ctx, handle)
	if err != nil {
		// Cancel raced ahead and tore the turn down (parked-turn teardown, or
		// the drive goroutine finishing) before we subscribed — Events then
		// returns ErrTurnNotFound + a nil iterator. The turn is gone, which is
		// exactly the clean cancel this test asserts, so don't range a nil.
		return
	}
	for ev := range events {
		if end, ok := ev.(turn.TurnEnd); ok && end.Reason == execution.OutcomeCanceled {
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
	dispatcher := mustTurn(turn.New(turnDeps(stub)))

	handle, err := dispatcher.Rehydrate(context.Background(), turn.RehydrateRequest{
		SessionID: "sess-restored",
		TurnID:    "turn-original",
		ProcessID: "process-42",
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
	events, err := dispatcher.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := dispatcher.Resume(ctx, handle, interrupts.Resolution{Approved: true}, nil); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	var sawDelta, sawEnd bool
	for ev := range events {
		switch e := ev.(type) {
		case turn.MessageDelta:
			sawDelta = true
		case turn.TurnEnd:
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
// Resume emits ErrorEvent + TurnEnd before returning its error.
func TestRehydrate_ResumeError_ReturnsError(t *testing.T) {
	stub := &stubEngine{runReply: "x", restoreResumeErr: errors.New("resume boom")}
	dispatcher := mustTurn(turn.New(turnDeps(stub)))

	handle, err := dispatcher.Rehydrate(context.Background(), turn.RehydrateRequest{
		SessionID: "sess-restored",
		TurnID:    "turn-original",
		ProcessID: "process-99",
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	events, err := dispatcher.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if err := dispatcher.Resume(context.Background(), handle, interrupts.Resolution{Approved: true}, nil); err == nil {
		t.Fatal("Resume returned nil error despite the restored process failure")
	}
	var sawError, sawEnd bool
	for ev := range events {
		switch ev.(type) {
		case turn.ErrorEvent:
			sawError = true
		case turn.TurnEnd:
			sawEnd = true
		}
	}
	if !sawError || !sawEnd {
		t.Fatalf("terminal stream = error:%v end:%v, want both", sawError, sawEnd)
	}
	if _, evErr := dispatcher.Events(context.Background(), handle); evErr == nil {
		t.Error("Events resolved a turn that should have been torn down")
	}
}

// TestStartTurn_ResolvesPerRunClient verifies a turn carrying a Model passes
// the resolver's client through to the engine's TurnRequest.ChatClient —
// the turn-dispatcher half of per-run model selection.
func TestStartTurn_ResolvesPerRunClient(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	sentinel, _ := chatclient.New(newCapturingModel())
	resolver := &fakeResolver{client: sentinel}

	dispatcher := mustTurn(turn.New(turnDeps(stub, withClientResolver(resolver))))
	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "hi",
		Provider:  "some-provider",
		Model:     "some-model",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)
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

// TestStartTurn_PassesCwd verifies the session's working directory flows
// from StartTurnRequest.Cwd through to the engine's TurnRequest.Cwd —
// the turn-dispatcher half of per-session tool working directories.
func TestStartTurn_PassesCwd(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}

	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "hi",
		Cwd:       "/work/project-a",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)
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

	dispatcher := mustTurn(turn.New(turnDeps(stub)))
	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "hi",
		Options:   &corechat.Options{Temperature: &temp},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := dispatcher.Events(ctx, handle)
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
	dispatcher := mustTurn(turn.New(turnDeps(engine)))

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
	interruptKinds := []string{"approval"}

	handle, err := dispatcher.StartTurn(context.Background(), turn.StartTurnRequest{
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
	interruptKinds[0] = "question"
	close(engine.release)

	events, err := dispatcher.Events(context.Background(), handle)
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
