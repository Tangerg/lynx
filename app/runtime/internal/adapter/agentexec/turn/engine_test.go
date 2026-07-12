package turn_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// TestStubEngineDrivesTurn — confirms the turn dispatcher runs a full
// turn against a stub engine, no real platform involved. If turn
// ever regrows a hard *agentexec.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	svc := mustTurn(turn.New(turnDeps(stub)))
	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "sess-1",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := svc.Events(ctx, handle)
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
	svc := mustTurn(turn.New(turnDeps(stub)))
	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{SessionID: "s", Message: "hi"})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := svc.Events(ctx, handle)
	for range events { //nolint:revive // drain to terminal (channel closes after endTurn)
	}
	proc := stub.lastProc.Load()
	if proc == nil {
		t.Fatal("stub engine never produced a process")
	}
	if !proc.discarded.Load() {
		t.Error("process not discarded at terminal teardown — snapshot would leak")
	}
}

// TestStubEngineBudgetStop — a turn whose process reports
// StoppedOnBudget ends with Reason=execution.OutcomeMaxBudget, not a plain
// completion, so clients can tell "stopped at the ceiling" apart from
// "model finished".
func TestStubEngineBudgetStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopOnBudget: true}
	svc := mustTurn(turn.New(turnDeps(stub)))

	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "go",
		MaxBudget: 1,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := svc.Events(ctx, handle)

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
	svc := mustTurn(turn.New(turnDeps(stub)))

	handle, _ := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "m",
	})
	if err := svc.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := svc.Events(ctx, handle)
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

// TestRehydrateResumesRestoredTurn covers the cross-restart path: a
// rehydrated turn restores its process via the engine, resumes it, and
// streams the continuation (delta + TurnEnd) on a fresh handle.
func TestRehydrateResumesRestoredTurn(t *testing.T) {
	stub := &stubEngine{runReply: "continuation reply"}
	svc := mustTurn(turn.New(turnDeps(stub)))

	handle, err := svc.Rehydrate(context.Background(), turn.RehydrateRequest{
		SessionID: "sess-restored",
		ProcessID: "proc-42",
		Approved:  true,
	})
	if err != nil {
		t.Fatalf("Rehydrate: %v", err)
	}
	if handle.TurnID == "" {
		t.Fatal("Rehydrate returned empty handle")
	}
	if got := stub.restoreCalls.Load(); got != 1 {
		t.Fatalf("RestoreTurn calls = %d, want 1", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, err := svc.Events(ctx, handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
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

// TestRehydrate_ResumeError_ReturnsError: when the restored process fails to
// resume, Rehydrate has already torn the turn down, so it must surface the
// error rather than hand back a handle to a dead turn (which would leave the
// caller's openSegment leaking ErrTurnNotFound instead of a clean run_not_found).
func TestRehydrate_ResumeError_ReturnsError(t *testing.T) {
	stub := &stubEngine{runReply: "x", restoreResumeErr: errors.New("resume boom")}
	svc := mustTurn(turn.New(turnDeps(stub)))

	handle, err := svc.Rehydrate(context.Background(), turn.RehydrateRequest{
		SessionID: "sess-restored",
		ProcessID: "proc-99",
		Approved:  true,
	})
	if err == nil {
		t.Fatal("Rehydrate returned nil error despite a failed resume; want the resume error surfaced")
	}
	if !errors.Is(err, turn.ErrRehydrateCommitted) {
		t.Fatalf("Rehydrate error = %v, want ErrRehydrateCommitted marker", err)
	}
	if handle.TurnID != "" {
		t.Errorf("Rehydrate returned a handle (%q) for a torn-down turn; want the zero handle", handle.TurnID)
	}
	// The torn-down turn must not linger in the registry.
	if _, evErr := svc.Events(context.Background(), turn.TurnHandle{TurnID: handle.TurnID}); evErr == nil {
		t.Error("Events resolved a turn that should have been torn down")
	}
}

// TestStartTurn_ResolvesPerRunClient verifies a turn carrying a Model passes
// the resolver's client through to the engine's TurnRequest.ChatClient —
// the turn-dispatcher half of per-run model selection.
func TestStartTurn_ResolvesPerRunClient(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	sentinel, _ := corechat.NewClient(newCapturingModel())
	resolver := &fakeResolver{client: sentinel}

	svc := mustTurn(turn.New(turnDeps(stub, withClientResolver(resolver))))
	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
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
	events, _ := svc.Events(ctx, handle)
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

	svc := mustTurn(turn.New(turnDeps(stub)))
	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "hi",
		Cwd:       "/work/project-a",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := svc.Events(ctx, handle)
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

	svc := mustTurn(turn.New(turnDeps(stub)))
	handle, err := svc.StartTurn(context.Background(), turn.StartTurnRequest{
		SessionID: "s",
		Message:   "hi",
		Options:   &corechat.Options{Temperature: &temp},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	events, _ := svc.Events(ctx, handle)
	for range events { // drain to TurnEnd
	}

	stub.mu.Lock()
	got := stub.lastOptions
	stub.mu.Unlock()
	if got == nil || got.Temperature == nil || *got.Temperature != 0.7 {
		t.Fatalf("engine options = %+v, want temperature 0.7", got)
	}
}
