package turn_test

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/chatclient"
	chatmodel "github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	planadapter "github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// turnDriver is the external-package test's view of a constructed controller.
// Production consumers each use smaller ports; these integration tests exercise
// the complete turn lifecycle.
type turnDriver interface {
	StartTurn(context.Context, runs.RootExecutionStart) (turn.Handle, error)
	PrepareTurn(context.Context, runs.RootExecutionStart) (turn.Handle, error)
	ActivateTurn(context.Context, turn.Handle) error
	Events(context.Context, turn.Handle) (iter.Seq[runs.ExecutorEvent], error)
	InjectSteering(context.Context, turn.Handle, []transcript.ContentBlock) error
	Resume(context.Context, turn.Handle, []agentexec.SuspensionAnswer, []interrupt.Kind) error
	ProcessID(context.Context, turn.Handle) (string, error)
	Rehydrate(context.Context, runs.RehydrateExecution) (turn.Handle, error)
	Cancel(context.Context, turn.Handle) error
	BeginShutdown()
	AwaitShutdown(context.Context) error
	ForgetSession(string)
}

func shutdownController(t testing.TB, controller interface {
	BeginShutdown()
	AwaitShutdown(context.Context) error
}) {
	t.Helper()
	controller.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := controller.AwaitShutdown(ctx); err != nil {
		t.Fatalf("shutdown turn controller: %v", err)
	}
}

func joinTurnCleanup(t testing.TB, controller interface {
	Cancel(context.Context, turn.Handle) error
}, handle turn.Handle) {
	t.Helper()
	err := controller.Cancel(context.Background(), handle)
	if err != nil && !errors.Is(err, turn.ErrTurnNotFound) {
		t.Fatalf("join terminal turn cleanup: %v", err)
	}
}

func testModelSelection(t testing.TB, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New(%q, %q): %v", provider, model, err)
	}
	return selection
}

func buildController(t *testing.T) (turnDriver, *agentexec.Engine) {
	t.Helper()

	model := newStubChatModel()
	client, err := chatclient.New(model, chatclient.Config{Defaults: *model.defaults})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := buildEngine(t, agentexec.Config{ChatClient: client})
	return mustTurn(turn.New(turnDeps(eng))), eng
}

func buildEngine(t *testing.T, cfg agentexec.Config) *agentexec.Engine {
	t.Helper()
	if cfg.DefaultCWD == "" {
		cfg.DefaultCWD = t.TempDir()
	}
	if cfg.UserHome == "" {
		cfg.UserHome = t.TempDir()
	}
	if cfg.Checkpoints == nil {
		cfg.Checkpoints = newMemoryCheckpointStore()
	}
	if cfg.BuildID == "" {
		cfg.BuildID = testProcessBuildID
	}
	var planStore planadapter.Store
	if cfg.Plan != nil {
		var ok bool
		planStore, ok = cfg.Plan.(planadapter.Store)
		if !ok {
			t.Fatalf("test engine plan source must support canonical Plan reads and replacements")
		}
	}
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{
		DefaultCWD: cfg.DefaultCWD,
		UserHome:   cfg.UserHome,
		Plan:       planStore,
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

func drainEvents(events iter.Seq[runs.ExecutorEvent]) []runs.ExecutionFact {
	var out []runs.ExecutionFact
	for ev := range events {
		if event, ok := ev.Payload.(runs.ExecutionFact); ok {
			out = append(out, event)
		}
	}
	return out
}

func eventNames(events []runs.ExecutionFact) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case runs.MessageDelta:
			out[i] = "MessageDelta"
		case runs.ToolCallStarted:
			out[i] = "ToolCallStarted"
		case runs.ToolCallFinished:
			out[i] = "ToolCallFinished"
		case runs.UsageReported:
			out[i] = "UsageReported"
		case runs.SegmentEnded:
			out[i] = "TurnEnd"
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
	return makeToolCall("shell", `{"command":"echo lyra","description":"Print lyra"}`)
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
	return makeToolCall("shell", `{"command":"echo lyra","description":"Print lyra"}`)
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
	defaults     *chatmodel.Options
	mu           sync.Mutex
	seenLengths  []int
	seenMessages [][]chatmodel.Message
}

func newHistoryAwareStub() *historyAwareStub {
	opts := &chatmodel.Options{Model: "stub-history"}
	return &historyAwareStub{defaults: opts}
}

func (m *historyAwareStub) DefaultOptions() chatmodel.Options { return *m.defaults }

func (m *historyAwareStub) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	m.mu.Lock()
	m.seenLengths = append(m.seenLengths, len(req.Messages))
	messages := make([]chatmodel.Message, len(req.Messages))
	for i, message := range req.Messages {
		messages[i] = message.Clone()
	}
	m.seenMessages = append(m.seenMessages, messages)
	m.mu.Unlock()
	return makeText("ok")
}

func (m *historyAwareStub) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func mustTurn(controller turnDriver, err error) turnDriver {
	if err != nil {
		panic(err)
	}
	return controller
}
