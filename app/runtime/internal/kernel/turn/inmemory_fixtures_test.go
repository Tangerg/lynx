package turn_test

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func buildDispatcher(t *testing.T) (turn.Dispatcher, *kernel.Engine) {
	t.Helper()

	client, err := chatmodel.NewClient(newStubChatModel())
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := buildEngine(t, kernel.Config{ChatClient: client})
	return mustTurn(turn.New(turn.Dependencies{Engine: eng})), eng
}

func buildEngine(t *testing.T, cfg kernel.Config) *kernel.Engine {
	t.Helper()
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{
		Workdir:         cfg.Workdir,
		SkillsGlobalDir: cfg.SkillsGlobalDir,
		Todos:           cfg.Todos,
	})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cfg.ToolResolver = built.Resolver
	cfg.Tools = built.Tools
	cfg.MCP = built.MCP
	cfg.Closers = built.Closers
	eng, err := kernel.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
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
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *stubChatModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

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
func (m *countingStubModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

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
		if msg.Type() == chatmodel.MessageTypeTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(text),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	calls := []*chatmodel.ToolCallPart{{ID: "c1", Name: name, Arguments: args}}
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(calls),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonToolCalls},
		},
		&chatmodel.ResponseMetadata{},
	)
}

type historyAwareStub struct {
	defaults    *chatmodel.Options
	mu          sync.Mutex
	seenLengths []int
}

func newHistoryAwareStub() *historyAwareStub {
	opts, _ := chatmodel.NewOptions("stub-history")
	return &historyAwareStub{defaults: opts}
}

func (m *historyAwareStub) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *historyAwareStub) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

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
