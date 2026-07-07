package turn_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// stubTurnProcess fakes the [kernel.TurnProcess] handle without
// touching the real platform. The done channel is pre-fired so
// runTurn receives immediately; status / output / cancel return
// the values the test wired.
type stubTurnProcess struct {
	id        string
	status    atomic.Int32 // core.AgentProcessStatus
	failure   error
	output    kernel.TurnOutput
	done      chan error
	onCancel  func()
	resumeErr error       // when set, Resume fails with it (covers the resume-error path)
	discarded atomic.Bool // set by Discard — asserts terminal snapshot cleanup
}

func newStubTurnProcess(id string, output kernel.TurnOutput) *stubTurnProcess {
	cp := &stubTurnProcess{
		id:     id,
		output: output,
		done:   make(chan error, 1),
	}
	cp.status.Store(int32(core.StatusCompleted))
	cp.done <- nil
	close(cp.done)
	return cp
}

func (cp *stubTurnProcess) ID() string { return cp.id }
func (cp *stubTurnProcess) Status() core.AgentProcessStatus {
	return core.AgentProcessStatus(cp.status.Load())
}
func (cp *stubTurnProcess) Failure() error     { return cp.failure }
func (cp *stubTurnProcess) Done() <-chan error { return cp.done }
func (cp *stubTurnProcess) Output() (kernel.TurnOutput, error) {
	return cp.output, nil
}
func (cp *stubTurnProcess) Cancel() error {
	cp.status.Store(int32(core.StatusKilled))
	if cp.onCancel != nil {
		cp.onCancel()
	}
	return nil
}

// Resume exists to satisfy engine.TurnProcess. Stubs never park on plan
// approval (Status stays Completed), so runTurn's resume loop doesn't
// call this — the real plan-mode path is covered by real-engine tests. Returns
// an already-fired done for safety.
func (cp *stubTurnProcess) Resume(_ context.Context, _ interrupts.Resolution) (<-chan error, error) {
	if cp.resumeErr != nil {
		return nil, cp.resumeErr
	}
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch, nil
}

// PendingAwaitable satisfies engine.TurnProcess. Stubs never park, so
// nothing is ever pending.
func (cp *stubTurnProcess) PendingAwaitable() core.Awaitable { return nil }

// Discard satisfies engine.TurnProcess. The stub holds no platform /
// snapshot; it just records that terminal cleanup ran.
func (cp *stubTurnProcess) Discard(_ context.Context) { cp.discarded.Store(true) }

// stubEngine satisfies the turn dispatcher's (unexported) engine
// dependency without touching the real platform / conversation history / MCP
// wiring. Existence proves the turn dispatcher does not depend on
// *kernel.Engine directly — only on the narrow interface.
type stubEngine struct {
	runTurnCalls     atomic.Int32
	restoreCalls     atomic.Int32
	runReply         string
	stopOnBudget     bool  // when true the produced TurnOutput sets StoppedOnBudget
	restoreResumeErr error // when set, a RestoreTurn'd process fails its Resume with it

	mu          sync.Mutex
	lastClient  core.ChatClient // captures RunTurnRequest.ChatClient
	lastCwd     string          // captures RunTurnRequest.Cwd
	lastCtx     context.Context // captures the ctx the engine runs under
	lastOptions *corechat.Options

	lastProc atomic.Pointer[stubTurnProcess] // the most recent process StartTurn handed back
}

func (s *stubEngine) StartTurn(ctx context.Context, req kernel.RunTurnRequest) kernel.TurnProcess {
	s.runTurnCalls.Add(1)
	s.mu.Lock()
	s.lastClient = req.ChatClient
	s.lastCwd = req.Cwd
	s.lastCtx = ctx
	s.lastOptions = req.Options.Clone()
	s.mu.Unlock()
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	proc := newStubTurnProcess("stub-proc-"+req.SessionID, kernel.TurnOutput{
		Reply:           s.runReply,
		StoppedOnBudget: s.stopOnBudget,
	})
	s.lastProc.Store(proc)
	return proc
}

// RestoreTurn simulates rebuilding a parked turn from a snapshot: it
// streams the continuation reply through the observer and returns a
// completed process, so Rehydrate's Resume → drive reaches TurnEnd.
func (s *stubEngine) RestoreTurn(_ context.Context, processID string, req kernel.RestoreTurnRequest) (kernel.TurnProcess, error) {
	s.restoreCalls.Add(1)
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	cp := newStubTurnProcess(processID, kernel.TurnOutput{Reply: s.runReply})
	cp.resumeErr = s.restoreResumeErr
	return cp, nil
}

func (s *stubEngine) InjectUserMessage(_ context.Context, _, _ string) error { return nil }

func (s *stubEngine) MaybeCompact(_ context.Context, _ string, _ func(context.Context) bool) (kernel.CompactionResult, error) {
	return kernel.CompactionResult{}, nil
}

func (s *stubEngine) MaybeExtract(_ context.Context, _, _ string) (kernel.ExtractionResult, error) {
	return kernel.ExtractionResult{}, nil
}

// TestStubEngineDrivesTurn — confirms the turn dispatcher runs a full
// turn against a stub engine, no real platform involved. If turn
// ever regrows a hard *kernel.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))
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
		t.Errorf("RunTurn called %d times, want 1", got)
	}
}

// TestDispatcher_DiscardsProcessOnTerminal verifies the turn discards its backing
// process at terminal teardown (endTurn → TurnProcess.Discard) — the seam that
// deletes the auto-snapshot. Without it every run leaks one process_snapshot
// row. The events channel closes only after endTurn runs, so reading the flag
// after the drain loop is race-free.
func TestDispatcher_DiscardsProcessOnTerminal(t *testing.T) {
	stub := &stubEngine{runReply: "done"}
	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))
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
// StoppedOnBudget ends with Reason=TurnEndBudgetExceeded, not a plain
// completion, so clients can tell "stopped at the ceiling" apart from
// "model finished".
func TestStubEngineBudgetStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopOnBudget: true}
	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))

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
			if end.Reason != turn.TurnEndBudgetExceeded {
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
	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))

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
		if end, ok := ev.(turn.TurnEnd); ok && end.Reason == turn.TurnEndCanceled {
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
	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))

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
			if e.Reason != turn.TurnEndCompleted {
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
	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))

	handle, err := svc.Rehydrate(context.Background(), turn.RehydrateRequest{
		SessionID: "sess-restored",
		ProcessID: "proc-99",
		Approved:  true,
	})
	if err == nil {
		t.Fatal("Rehydrate returned nil error despite a failed resume; want the resume error surfaced")
	}
	if handle.TurnID != "" {
		t.Errorf("Rehydrate returned a handle (%q) for a torn-down turn; want the zero handle", handle.TurnID)
	}
	// The torn-down turn must not linger in the registry.
	if _, evErr := svc.Events(context.Background(), turn.TurnHandle{TurnID: handle.TurnID}); evErr == nil {
		t.Error("Events resolved a turn that should have been torn down")
	}
}

// slowStubEngine simulates an engine that respects ctx cancellation
// without ever returning normally — the stub TurnProcess holds a
// done channel that fires only when ctx is canceled, mirroring how
// the real platform reacts to KillProcess / ctx cancel.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartTurn(ctx context.Context, _ kernel.RunTurnRequest) kernel.TurnProcess {
	cp := &stubTurnProcess{
		id:   "slow-stub-proc",
		done: make(chan error, 1),
	}
	cp.status.Store(int32(core.StatusRunning))
	cp.onCancel = func() {
		select {
		case cp.done <- errors.New("canceled"):
		default:
		}
	}
	go func() {
		<-ctx.Done()
		select {
		case cp.done <- errors.New("canceled"):
		default:
		}
	}()
	return cp
}

// fakeResolver returns a preset client, recording the (provider, model) it
// was asked to resolve.
type fakeResolver struct {
	client      *corechat.Client
	gotProvider string
	gotModel    string
	resolveErr  error
}

func (r *fakeResolver) ResolveClient(_ context.Context, provider, model string) (*corechat.Client, error) {
	r.gotProvider, r.gotModel = provider, model
	if r.resolveErr != nil {
		return nil, r.resolveErr
	}
	return r.client, nil
}

// TestStartTurn_ResolvesPerRunClient verifies a turn carrying a Model passes
// the resolver's client through to the engine's RunTurnRequest.ChatClient —
// the turn-dispatcher half of per-run model selection.
func TestStartTurn_ResolvesPerRunClient(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	sentinel, _ := corechat.NewClient(newCapturingModel())
	resolver := &fakeResolver{client: sentinel}

	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub, ClientResolver: resolver}))
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
// from StartTurnRequest.Cwd through to the engine's RunTurnRequest.Cwd —
// the turn-dispatcher half of per-session tool working directories.
func TestStartTurn_PassesCwd(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}

	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))
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

	svc := mustTurn(turn.New(turn.Dependencies{Engine: stub}))
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

// capturingModel is a minimal chat.Model for building a sentinel client.
type capturingModel struct{ defaults *corechat.Options }

func newCapturingModel() *capturingModel {
	opts, _ := corechat.NewOptions("sentinel-model")
	return &capturingModel{defaults: opts}
}
func (m *capturingModel) DefaultOptions() corechat.Options { return *m.defaults }
func (m *capturingModel) Metadata() corechat.ModelMetadata {
	return corechat.ModelMetadata{Provider: "stub"}
}
func (m *capturingModel) Call(_ context.Context, _ *corechat.Request) (*corechat.Response, error) {
	return corechat.NewResponse(&corechat.Result{
		AssistantMessage: corechat.NewAssistantMessage("ok"),
		Metadata:         &corechat.ResultMetadata{FinishReason: corechat.FinishReasonStop},
	}, &corechat.ResponseMetadata{})
}
func (m *capturingModel) Stream(ctx context.Context, req *corechat.Request) iter.Seq2[*corechat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*corechat.Response, error) bool) { yield(resp, err) }
}
