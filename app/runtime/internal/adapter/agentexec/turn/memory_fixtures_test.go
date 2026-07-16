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
)

func buildDispatcher(t *testing.T) (turn.Dispatcher, *agentexec.Engine) {
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
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{
		Workdir: cfg.Workdir,
		Todos:   cfg.Todos,
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

func drainEvents(events iter.Seq[turn.Event]) []turn.Event {
	var out []turn.Event
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func eventNames(events []turn.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case turn.TurnStart:
			out[i] = "TurnStart"
		case turn.MessageDelta:
			out[i] = "MessageDelta"
		case turn.ToolCallStart:
			out[i] = "ToolCallStart"
		case turn.ToolCallEnd:
			out[i] = "ToolCallEnd"
		case turn.TurnEnd:
			out[i] = "TurnEnd"
		case turn.ErrorEvent:
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

func baseSeq(ev turn.Event) uint64 {
	switch e := ev.(type) {
	case turn.TurnStart:
		return e.Seq
	case turn.MessageDelta:
		return e.Seq
	case turn.ToolCallStart:
		return e.Seq
	case turn.ToolCallEnd:
		return e.Seq
	case turn.TurnEnd:
		return e.Seq
	case turn.ErrorEvent:
		return e.Seq
	}
	return 0
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

func mustTurn(dispatcher turn.Dispatcher, err error) turn.Dispatcher {
	if err != nil {
		panic(err)
	}
	return dispatcher
}
