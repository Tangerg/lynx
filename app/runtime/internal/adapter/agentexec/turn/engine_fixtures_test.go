package turn_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/chatclient"
	corechat "github.com/Tangerg/lynx/core/chat"
)

type testEngine interface {
	StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error)
	RestoreTurn(ctx context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error)
}

func turnDeps(engine testEngine, opts ...func(*turn.Dependencies)) turn.Dependencies {
	services := noopTurnServices{}
	deps := turn.Dependencies{
		Engine:    engine,
		Steering:  services,
		Compactor: services,
		Extractor: services,
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

func mustApprovalPolicy(t testing.TB, mode approval.Mode, store approval.RuleStore) approval.Policy {
	t.Helper()
	policy, err := approval.New(mode, store)
	if err != nil {
		t.Fatalf("new approval policy: %v", err)
	}
	return policy
}

func withClientResolver(resolver interface {
	ResolveClient(ctx context.Context, provider, model string) (*chatclient.Client, error)
}) func(*turn.Dependencies) {
	return func(deps *turn.Dependencies) {
		deps.ClientResolver = resolver
	}
}

// stubTurnProcess fakes the agentexec.TurnProcess handle without touching the real
// engine. The done channel is pre-fired so runTurn receives immediately;
// status / output / cancel return the values the test wired.
type stubTurnProcess struct {
	id         string
	status     atomic.Int32 // core.ProcessStatus
	failure    error
	output     agentexec.TurnOutput
	done       chan error
	onCancel   func()
	resumeErr  error       // when set, Resume fails with it
	discardErr error       // returned by Discard to verify teardown observability
	discarded  atomic.Bool // set by Discard to assert terminal snapshot cleanup
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
func (cp *stubTurnProcess) Status() core.ProcessStatus {
	return core.ProcessStatus(cp.status.Load())
}
func (cp *stubTurnProcess) Failure() error     { return cp.failure }
func (cp *stubTurnProcess) Done() <-chan error { return cp.done }
func (cp *stubTurnProcess) Output() (agentexec.TurnOutput, error) {
	return cp.output, nil
}
func (cp *stubTurnProcess) Cancel(context.Context) error {
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

func (cp *stubTurnProcess) Suspension() *agent.Suspension { return nil }

func (cp *stubTurnProcess) Discard(_ context.Context) error {
	cp.discarded.Store(true)
	return cp.discardErr
}

// stubEngine satisfies the turn dispatcher's engine dependency without touching
// the real engine, conversation history, or MCP wiring.
type stubEngine struct {
	runTurnCalls     atomic.Int32
	restoreCalls     atomic.Int32
	runReply         string
	stopReason       agentexec.StopReason
	restoreResumeErr error
	discardErr       error

	mu                   sync.Mutex
	lastClient           *chatclient.Client
	lastCwd              string
	lastCtx              context.Context
	lastOptions          *corechat.Options
	restoreGateTool      string
	restoreGateArguments string
	restoreGateVerdict   agentexec.ToolApprovalVerdict

	lastProcess atomic.Pointer[stubTurnProcess]
}

func (s *stubEngine) StartTurn(ctx context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	s.runTurnCalls.Add(1)
	s.mu.Lock()
	s.lastClient = request.ChatClient
	s.lastCwd = request.Cwd
	s.lastCtx = ctx
	if request.Options == nil {
		s.lastOptions = nil
	} else {
		copy := *request.Options
		copy.Stop = append([]string(nil), request.Options.Stop...)
		s.lastOptions = &copy
	}
	s.mu.Unlock()
	if request.Observer != nil {
		request.Observer.OnMessageDelta(s.runReply)
	}
	process := newStubTurnProcess("stub-processess-"+request.SessionID, agentexec.TurnOutput{
		Reply:      s.runReply,
		StopReason: s.stopReason,
	})
	process.discardErr = s.discardErr
	s.lastProcess.Store(process)
	return process, nil
}

func (s *stubEngine) RestoreTurn(_ context.Context, processID string, request agentexec.RestoreTurnRequest) (agentexec.TurnProcess, error) {
	s.restoreCalls.Add(1)
	if request.Observer != nil && s.restoreGateTool != "" {
		verdict := request.Observer.ApproveToolCall(
			context.Background(), "restore-call", s.restoreGateTool, s.restoreGateArguments, agentexec.ToolApprovalTarget{},
		)
		s.mu.Lock()
		s.restoreGateVerdict = verdict
		s.mu.Unlock()
	}
	if request.Observer != nil {
		request.Observer.OnMessageDelta(s.runReply)
	}
	cp := newStubTurnProcess(processID, agentexec.TurnOutput{Reply: s.runReply})
	cp.resumeErr = s.restoreResumeErr
	return cp, nil
}

type noopTurnServices struct{}

func (noopTurnServices) InjectUser(context.Context, string, string) error { return nil }

func (noopTurnServices) MaybeCompact(context.Context, string, int, func(context.Context) bool) (turn.CompactionResult, error) {
	return turn.CompactionResult{}, nil
}

func (noopTurnServices) MaybeExtract(context.Context, string, string) error { return nil }

// slowStubEngine simulates an engine that respects ctx cancellation without
// ever returning normally.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartTurn(ctx context.Context, _ agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	cp := &stubTurnProcess{
		id:   "slow-stub-processess",
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
	return cp, nil
}

type capturedTurnRequest struct {
	temperature      float64
	frequencyPenalty float64
	topK             int64
	stop             string
	mediaByte        byte
}

type delayedCaptureEngine struct {
	stubEngine
	entered  chan struct{}
	release  chan struct{}
	captured chan capturedTurnRequest
}

func newDelayedCaptureEngine() *delayedCaptureEngine {
	return &delayedCaptureEngine{
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		captured: make(chan capturedTurnRequest, 1),
	}
}

func (e *delayedCaptureEngine) StartTurn(_ context.Context, request agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	close(e.entered)
	<-e.release

	captured := capturedTurnRequest{}
	if request.Options != nil {
		if request.Options.Temperature != nil {
			captured.temperature = *request.Options.Temperature
		}
		if request.Options.FrequencyPenalty != nil {
			captured.frequencyPenalty = *request.Options.FrequencyPenalty
		}
		if request.Options.TopK != nil {
			captured.topK = *request.Options.TopK
		}
		if len(request.Options.Stop) > 0 {
			captured.stop = request.Options.Stop[0]
		}
	}
	if len(request.Media) > 0 && request.Media[0] != nil && len(request.Media[0].Source.Bytes) > 0 {
		captured.mediaByte = request.Media[0].Source.Bytes[0]
	}
	e.captured <- captured
	return newStubTurnProcess("delayed-capture", agentexec.TurnOutput{Reply: "ok"}), nil
}

type immediateStartFailureEngine struct {
	stubEngine
	err error
}

func (e *immediateStartFailureEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	return nil, e.err
}

type blockedStartFailureEngine struct {
	stubEngine
	err     error
	entered chan struct{}
	release chan struct{}
}

func newBlockedStartFailureEngine(err error) *blockedStartFailureEngine {
	return &blockedStartFailureEngine{
		err:     err,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *blockedStartFailureEngine) StartTurn(context.Context, agentexec.TurnRequest) (agentexec.TurnProcess, error) {
	close(e.entered)
	<-e.release
	return nil, e.err
}

// fakeResolver returns a preset client, recording the provider/model it was
// asked to resolve.
type fakeResolver struct {
	client      *chatclient.Client
	gotProvider string
	gotModel    string
	resolveErr  error
}

func (r *fakeResolver) ResolveClient(_ context.Context, provider, model string) (*chatclient.Client, error) {
	r.gotProvider, r.gotModel = provider, model
	if r.resolveErr != nil {
		return nil, r.resolveErr
	}
	return r.client, nil
}

// capturingModel is a minimal chat.Model for building a sentinel client.
type capturingModel struct{ defaults *corechat.Options }

func newCapturingModel() *capturingModel {
	opts := &corechat.Options{Model: "sentinel-model"}
	return &capturingModel{defaults: opts}
}
func (m *capturingModel) DefaultOptions() corechat.Options { return *m.defaults }
func (m *capturingModel) Call(_ context.Context, _ *corechat.Request) (*corechat.Response, error) {
	message := corechat.NewAssistantMessage(corechat.NewTextPart("ok"))
	return corechat.NewResponse(corechat.Choice{Index: 0, Message: &message, FinishReason: corechat.FinishReasonStop})
}
func (m *capturingModel) Stream(ctx context.Context, request *corechat.Request) iter.Seq2[*corechat.Response, error] {
	resp, err := m.Call(ctx, request)
	return func(yield func(*corechat.Response, error) bool) { yield(resp, err) }
}
