package turn_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	chatmodel "github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/todotool"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// turnDriver is the external-package test's view of a constructed dispatcher.
// Production consumers each use smaller ports; these integration tests exercise
// the complete turn lifecycle.
type turnDriver interface {
	StartTurn(context.Context, turn.StartTurnRequest) (turn.TurnHandle, error)
	PrepareTurn(context.Context, turn.StartTurnRequest) (turn.TurnHandle, error)
	ActivateTurn(context.Context, turn.TurnHandle) error
	Events(context.Context, turn.TurnHandle) (iter.Seq[runs.EngineEvent], error)
	InjectSteering(context.Context, turn.TurnHandle, string) error
	Resume(context.Context, turn.TurnHandle, interrupts.Resolution, []string) error
	ProcessID(context.Context, turn.TurnHandle) (string, error)
	Rehydrate(context.Context, turn.RehydrateRequest) (turn.TurnHandle, error)
	Cancel(context.Context, turn.TurnHandle) error
	Close() error
	ForgetSession(string)
}

func buildDispatcher(t *testing.T) (turnDriver, *agentexec.Engine) {
	t.Helper()

	model := newStubChatModel()
	client, err := chatclient.New(model, chatclient.WithDefaults(*model.defaults))
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	return mustTurn(turn.New(turnDeps(eng))), eng
}

func buildEngine(t *testing.T, cfg agentexec.Config) *agentexec.Engine {
	t.Helper()
	var todos todotool.Store
	if cfg.Todos != nil {
		var ok bool
		todos, ok = cfg.Todos.(todotool.Store)
		if !ok {
			t.Fatalf("test engine todo source must support todo_write")
		}
	}
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{
		Workdir: cfg.Workdir,
		Todos:   todos,
	})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupToolEnvironment(t, built)
	cfg.ToolResolver = built.Resolver
	eng, err := agentexec.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return eng
}

func cleanupToolEnvironment(t *testing.T, built toolset.Built) {
	t.Helper()
	t.Cleanup(func() {
		for index := len(built.Closers) - 1; index >= 0; index-- {
			if closeFn := built.Closers[index]; closeFn != nil {
				_ = closeFn()
			}
		}
	})
}

func drainEvents(events iter.Seq[runs.EngineEvent]) []runs.EngineEvent {
	var out []runs.EngineEvent
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func eventNames(events []runs.EngineEvent) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case runs.TurnStart:
			out[i] = "TurnStart"
		case runs.MessageDelta:
			out[i] = "MessageDelta"
		case runs.ToolCallStart:
			out[i] = "ToolCallStart"
		case runs.ToolCallEnd:
			out[i] = "ToolCallEnd"
		case runs.TurnEnd:
			out[i] = "TurnEnd"
		case runs.ErrorEvent:
			out[i] = "ErrorEvent"
		default:
			out[i] = "?"
		}
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type stubChatModel struct{ defaults *chatmodel.Options }

func newStubChatModel() *stubChatModel {
	opts := &chatmodel.Options{Model: "stub-model"}
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options { return *m.defaults }

func (m *stubChatModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("shell", `{"command":"echo lyra"}`)
}

func (m *stubChatModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

type countingStubModel struct {
	defaults *chatmodel.Options
	calls    atomic.Int32
}

func (m *countingStubModel) DefaultOptions() chatmodel.Options { return *m.defaults }

func (m *countingStubModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.calls.Add(1)
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("shell", `{"command":"echo lyra"}`)
}

func (m *countingStubModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func hasToolMsg(messages []chatmodel.Message) bool {
	for _, msg := range messages {
		if msg.Role == chatmodel.RoleTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	message := chatmodel.NewAssistantMessage(chatmodel.NewTextPart(text))
	return chatmodel.NewResponse(chatmodel.Choice{Index: 0, Message: &message, FinishReason: chatmodel.FinishReasonStop})
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	message := chatmodel.NewAssistantMessage(chatmodel.NewToolCallPart(chatmodel.ToolCall{ID: "c1", Name: name, Arguments: args}))
	return chatmodel.NewResponse(chatmodel.Choice{Index: 0, Message: &message, FinishReason: chatmodel.FinishReasonToolCalls})
}

type historyAwareStub struct {
	defaults    *chatmodel.Options
	mu          sync.Mutex
	seenLengths []int
}

func newHistoryAwareStub() *historyAwareStub {
	opts := &chatmodel.Options{Model: "stub-history"}
	return &historyAwareStub{defaults: opts}
}

func (m *historyAwareStub) DefaultOptions() chatmodel.Options { return *m.defaults }

func (m *historyAwareStub) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.mu.Lock()
	m.seenLengths = append(m.seenLengths, len(req.Messages))
	m.mu.Unlock()
	return makeText("ok")
}

func (m *historyAwareStub) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func mustTurn(dispatcher turnDriver, err error) turnDriver {
	if err != nil {
		panic(err)
	}
	return dispatcher
}
