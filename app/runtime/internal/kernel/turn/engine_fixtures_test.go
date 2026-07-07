package turn_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

// stubTurnProcess fakes the kernel.TurnProcess handle without touching the real
// platform. The done channel is pre-fired so runTurn receives immediately;
// status / output / cancel return the values the test wired.
type stubTurnProcess struct {
	id        string
	status    atomic.Int32 // core.AgentProcessStatus
	failure   error
	output    kernel.TurnOutput
	done      chan error
	onCancel  func()
	resumeErr error       // when set, Resume fails with it
	discarded atomic.Bool // set by Discard to assert terminal snapshot cleanup
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

func (cp *stubTurnProcess) Resume(_ context.Context, _ interrupts.Resolution) (<-chan error, error) {
	if cp.resumeErr != nil {
		return nil, cp.resumeErr
	}
	ch := make(chan error, 1)
	ch <- nil
	close(ch)
	return ch, nil
}

func (cp *stubTurnProcess) PendingAwaitable() core.Awaitable { return nil }

func (cp *stubTurnProcess) Discard(_ context.Context) { cp.discarded.Store(true) }

// stubEngine satisfies the turn dispatcher's engine dependency without touching
// the real platform, conversation history, or MCP wiring.
type stubEngine struct {
	runTurnCalls     atomic.Int32
	restoreCalls     atomic.Int32
	runReply         string
	stopOnBudget     bool
	restoreResumeErr error

	mu          sync.Mutex
	lastClient  core.ChatClient
	lastCwd     string
	lastCtx     context.Context
	lastOptions *corechat.Options

	lastProc atomic.Pointer[stubTurnProcess]
}

func (s *stubEngine) StartTurn(ctx context.Context, req kernel.TurnRequest) kernel.TurnProcess {
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

// slowStubEngine simulates an engine that respects ctx cancellation without
// ever returning normally.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartTurn(ctx context.Context, _ kernel.TurnRequest) kernel.TurnProcess {
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

// fakeResolver returns a preset client, recording the provider/model it was
// asked to resolve.
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
