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
	corechat "github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/kernel"
	"github.com/Tangerg/lynx/lyra/internal/kernel/turn"
)

// stubChatProcess fakes the [kernel.ChatProcess] handle without
// touching the real platform. The done channel is pre-fired so
// runTurn receives immediately; status / output / cancel return
// the values the test wired.
type stubChatProcess struct {
	id        string
	status    atomic.Int32 // core.AgentProcessStatus
	failure   error
	output    kernel.ChatOutput
	done      chan error
	onCancel  func()
	discarded atomic.Bool // set by Discard — asserts terminal snapshot cleanup
}

func newStubChatProcess(id string, output kernel.ChatOutput) *stubChatProcess {
	cp := &stubChatProcess{
		id:     id,
		output: output,
		done:   make(chan error, 1),
	}
	cp.status.Store(int32(core.StatusCompleted))
	cp.done <- nil
	close(cp.done)
	return cp
}

func (cp *stubChatProcess) ID() string { return cp.id }
func (cp *stubChatProcess) Status() core.AgentProcessStatus {
	return core.AgentProcessStatus(cp.status.Load())
}
func (cp *stubChatProcess) Failure() error     { return cp.failure }
func (cp *stubChatProcess) Done() <-chan error { return cp.done }
func (cp *stubChatProcess) Output() (kernel.ChatOutput, error) {
	return cp.output, nil
}
func (cp *stubChatProcess) Cancel() error {
	cp.status.Store(int32(core.StatusKilled))
	if cp.onCancel != nil {
		cp.onCancel()
	}
	return nil
}

// Resume exists to satisfy engine.ChatProcess. Stubs never park on plan
// approval (Status stays Completed), so runTurn's resume loop doesn't
// call this — the real plan-mode path is covered by buildPlanService's
// real-engine tests. Returns an already-fired done for safety.
func (cp *stubChatProcess) Resume(_ context.Context, _ interrupts.Resolution) (<-chan error, error) {
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch, nil
}

// PendingAwaitable satisfies engine.ChatProcess. Stubs never park, so
// nothing is ever pending.
func (cp *stubChatProcess) PendingAwaitable() core.Awaitable { return nil }

// Discard satisfies engine.ChatProcess. The stub holds no platform /
// snapshot; it just records that terminal cleanup ran.
func (cp *stubChatProcess) Discard(_ context.Context) { cp.discarded.Store(true) }

// stubEngine satisfies the turn service's (unexported) engine
// dependency without touching the real platform / chat-memory / MCP
// wiring. Existence proves the turn service does not depend on
// *kernel.Engine directly — only on the narrow interface.
type stubEngine struct {
	runChatCalls atomic.Int32
	restoreCalls atomic.Int32
	runReply     string
	stopOnBudget bool // when true the produced ChatOutput sets StoppedOnBudget

	mu         sync.Mutex
	lastClient *corechat.Client // captures RunChatRequest.ChatClient
	lastCwd    string           // captures RunChatRequest.Cwd
	lastCtx    context.Context  // captures the ctx the engine runs under

	lastProc atomic.Pointer[stubChatProcess] // the most recent process StartChat handed back
}

func (s *stubEngine) StartChat(ctx context.Context, req kernel.RunChatRequest) kernel.ChatProcess {
	s.runChatCalls.Add(1)
	s.mu.Lock()
	s.lastClient = req.ChatClient
	s.lastCwd = req.Cwd
	s.lastCtx = ctx
	s.mu.Unlock()
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	proc := newStubChatProcess("stub-proc-"+req.SessionID, kernel.ChatOutput{
		Reply:           s.runReply,
		StoppedOnBudget: s.stopOnBudget,
	})
	s.lastProc.Store(proc)
	return proc
}

// RestoreChat simulates rebuilding a parked turn from a snapshot: it
// streams the continuation reply through the observer and returns a
// completed process, so Rehydrate's Resume → drive reaches TurnEnd.
func (s *stubEngine) RestoreChat(_ context.Context, processID string, req kernel.RestoreChatRequest) (kernel.ChatProcess, error) {
	s.restoreCalls.Add(1)
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	return newStubChatProcess(processID, kernel.ChatOutput{Reply: s.runReply}), nil
}

func (s *stubEngine) InjectUserMessage(_ context.Context, _, _ string) error { return nil }

func (s *stubEngine) MaybeCompact(_ context.Context, _ string) (kernel.CompactionResult, error) {
	return kernel.CompactionResult{}, nil
}

func (s *stubEngine) MaybeExtract(_ context.Context, _, _ string) (kernel.ExtractionResult, error) {
	return kernel.ExtractionResult{}, nil
}

// TestStubEngineDrivesTurn — confirms the turn service runs a full
// turn against a stub engine, no real platform involved. If turn
// ever regrows a hard *kernel.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	svc := mustChat(turn.New(stub, nil, nil))
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
	if got := stub.runChatCalls.Load(); got != 1 {
		t.Errorf("RunChat called %d times, want 1", got)
	}
}

// TestService_DiscardsProcessOnTerminal verifies the turn discards its backing
// process at terminal teardown (endTurn → ChatProcess.Discard) — the seam that
// deletes the auto-snapshot. Without it every run leaks one process_snapshot
// row. The events channel closes only after endTurn runs, so reading the flag
// after the drain loop is race-free.
func TestService_DiscardsProcessOnTerminal(t *testing.T) {
	stub := &stubEngine{runReply: "done"}
	svc := mustChat(turn.New(stub, nil, nil))
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
	svc := mustChat(turn.New(stub, nil, nil))

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
	svc := mustChat(turn.New(stub, nil, nil))

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
	svc := mustChat(turn.New(stub, nil, nil))

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
		t.Fatalf("RestoreChat calls = %d, want 1", got)
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

// slowStubEngine simulates an engine that respects ctx cancellation
// without ever returning normally — the stub ChatProcess holds a
// done channel that fires only when ctx is canceled, mirroring how
// the real platform reacts to KillProcess / ctx cancel.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartChat(ctx context.Context, _ kernel.RunChatRequest) kernel.ChatProcess {
	cp := &stubChatProcess{
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
// the resolver's client through to the engine's RunChatRequest.ChatClient —
// the turn-service half of per-run model selection.
func TestStartTurn_ResolvesPerRunClient(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}
	sentinel, _ := corechat.NewClient(newCapturingModel())
	resolver := &fakeResolver{client: sentinel}

	svc := mustChat(turn.New(stub, nil, resolver))
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
// from StartTurnRequest.Cwd through to the engine's RunChatRequest.Cwd —
// the turn-service half of per-session tool working directories.
func TestStartTurn_PassesCwd(t *testing.T) {
	stub := &stubEngine{runReply: "ok"}

	svc := mustChat(turn.New(stub, nil, nil))
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
