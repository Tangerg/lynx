package turn_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	corechat "github.com/Tangerg/lynx/core/model/chat"
)

type testEngine interface {
	StartTurn(ctx context.Context, req agentexec.TurnRequest) agentexec.TurnProcess
	RestoreTurn(ctx context.Context, processID string, req agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
	InjectUserMessage(ctx context.Context, sessionID, text string) error
	MaybeCompact(ctx context.Context, sessionID string, preCompact func(context.Context) bool) (agentexec.CompactionResult, error)
	MaybeExtract(ctx context.Context, sessionID, cwd string) (agentexec.ExtractionResult, error)
}

func turnDeps(engine testEngine, opts ...func(*turn.Dependencies)) turn.Dependencies {
	deps := turn.Dependencies{
		Engine: engine,
	}
	for _, opt := range opts {
		opt(&deps)
	}
	return deps
}

func withApproval(policy approval.Policy) func(*turn.Dependencies) {
	return func(deps *turn.Dependencies) {
		deps.Approval = policy
	}
}

func withClientResolver(resolver interface {
	ResolveClient(ctx context.Context, provider, model string) (*corechat.Client, error)
}) func(*turn.Dependencies) {
	return func(deps *turn.Dependencies) {
		deps.ClientResolver = resolver
	}
}

// stubTurnProcess fakes the agentexec.TurnProcess handle without touching the real
// platform. The done channel is pre-fired so runTurn receives immediately;
// status / output / cancel return the values the test wired.
type stubTurnProcess struct {
	id        string
	status    atomic.Int32 // core.AgentProcessStatus
	failure   error
	output    agentexec.TurnOutput
	done      chan error
	onCancel  func()
	resumeErr error       // when set, Resume fails with it
	discarded atomic.Bool // set by Discard to assert terminal snapshot cleanup
}

func newStubTurnProcess(id string, output agentexec.TurnOutput) *stubTurnProcess {
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
func (cp *stubTurnProcess) Output() (agentexec.TurnOutput, error) {
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

func (s *stubEngine) StartTurn(ctx context.Context, req agentexec.TurnRequest) agentexec.TurnProcess {
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
	proc := newStubTurnProcess("stub-proc-"+req.SessionID, agentexec.TurnOutput{
		Reply:           s.runReply,
		StoppedOnBudget: s.stopOnBudget,
	})
	s.lastProc.Store(proc)
	return proc
}

func (s *stubEngine) RestoreTurn(_ context.Context, processID string, req agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	s.restoreCalls.Add(1)
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	cp := newStubTurnProcess(processID, agentexec.TurnOutput{Reply: s.runReply})
	cp.resumeErr = s.restoreResumeErr
	return cp, nil
}

func (s *stubEngine) InjectUserMessage(_ context.Context, _, _ string) error { return nil }

func (s *stubEngine) MaybeCompact(_ context.Context, _ string, _ func(context.Context) bool) (agentexec.CompactionResult, error) {
	return agentexec.CompactionResult{}, nil
}

func (s *stubEngine) MaybeExtract(_ context.Context, _, _ string) (agentexec.ExtractionResult, error) {
	return agentexec.ExtractionResult{}, nil
}

// slowStubEngine simulates an engine that respects ctx cancellation without
// ever returning normally.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartTurn(ctx context.Context, _ agentexec.TurnRequest) agentexec.TurnProcess {
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
