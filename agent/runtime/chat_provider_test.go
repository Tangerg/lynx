package runtime_test

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

// recordingModel is a chat.Model that flips a flag when it's called, so a
// test can tell which client a turn actually routed through.
type recordingModel struct {
	called bool
}

func newRecordingModel() *recordingModel { return &recordingModel{} }

func (m *recordingModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	m.called = true
	message := chat.NewAssistantMessage(chat.NewTextPart("ok"))
	resp, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return resp, nil
}

func (m *recordingModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// fixedClientProvider is a ChatProvider that always hands back one
// preset client for per-process overrides selected by the caller.
type fixedClientProvider struct {
	name   string
	client *chatclient.Client
}

func (f fixedClientProvider) Name() string { return f.name }
func (f fixedClientProvider) Chat(core.ProcessView) core.ChatCapability {
	if f.client == nil {
		return core.ChatCapability{}
	}
	return core.ChatCapability{Model: f.client, Streamer: f.client}
}

var _ core.ChatProvider = fixedClientProvider{}

type callIn struct{ V int }
type callOut struct{ V int }

// callsChat is an action body that issues one LLM call through whichever
// client the ProcessContext resolved — the observable for which client won.
func callsChat(t *testing.T) func(context.Context, *core.ProcessContext, callIn) (callOut, error) {
	return func(ctx context.Context, pc *core.ProcessContext, in callIn) (callOut, error) {
		capability, err := pc.Chat()
		if err != nil {
			return callOut{}, err
		}
		request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hi")))
		if _, err := capability.Model.Call(ctx, request); err != nil {
			t.Errorf("chat call: %v", err)
		}
		return callOut{V: in.V + 1}, nil
	}
}

func chatAgent(t *testing.T) *core.Agent {
	return agent.New(agent.AgentConfig{Name: "chat-router", Actions: []agent.Action{agent.NewAction("call", callsChat(t), core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[callOut](core.GoalConfig{Description: "done"})}})
}

// TestChatProvider_OverridesEngineClient verifies a per-process
// ChatProvider extension wins over the engine's shared client — the
// mechanism that lets one Engine serve turns against different models.
func TestChatProvider_OverridesEngineClient(t *testing.T) {
	platformModel := newRecordingModel()
	overrideModel := newRecordingModel()
	platformClient, _ := chatclient.New(platformModel)
	overrideClient, _ := chatclient.New(overrideModel)

	a := chatAgent(t)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: platformClient, Streamer: platformClient}})
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	_, err := engine.Run(
		t.Context(), a,
		core.Input(callIn{V: 1}),
		core.ProcessOptions{
			Extensions: []core.Extension{
				fixedClientProvider{name: "per-run-model", client: overrideClient},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !overrideModel.called {
		t.Error("override client was not used")
	}
	if platformModel.called {
		t.Error("engine client was used despite a ChatProvider override")
	}
}

// TestChatProvider_FallsBackToEngine verifies that with no provider
// registered (or one that returns nil), the engine's client is used.
func TestChatProvider_FallsBackToEngine(t *testing.T) {
	platformModel := newRecordingModel()
	platformClient, _ := chatclient.New(platformModel)

	a := chatAgent(t)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: platformClient, Streamer: platformClient}})
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// A provider that defers (nil) must not shadow the engine client.
	_, err := engine.Run(
		t.Context(), a,
		core.Input(callIn{V: 1}),
		core.ProcessOptions{
			Extensions: []core.Extension{
				fixedClientProvider{name: "defers", client: nil},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !platformModel.called {
		t.Error("engine client was not used as the fallback")
	}
}

func TestChatProvider_TypedNilFallsBackToEngine(t *testing.T) {
	platformModel := newRecordingModel()
	platformClient, _ := chatclient.New(platformModel)
	var typedNil *chatclient.Client

	a := chatAgent(t)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: platformClient, Streamer: platformClient}})
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	_, err := engine.Run(
		t.Context(), a,
		core.Input(callIn{V: 1}),
		core.ProcessOptions{
			Extensions: []core.Extension{
				fixedClientProvider{name: "typed-nil", client: typedNil},
			},
		},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !platformModel.called {
		t.Error("engine client was not used when provider returned a typed nil")
	}
}

type streamingOnlyProvider struct{}

func (streamingOnlyProvider) Name() string { return "streaming-only" }
func (streamingOnlyProvider) Chat(core.ProcessView) core.ChatCapability {
	return core.ChatCapability{
		Streamer: chat.StreamerFunc(func(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
			return func(func(*chat.Response, error) bool) {}
		}),
	}
}

type panickingChatProvider struct{ cause error }

func (panickingChatProvider) Name() string { return "panic-chat" }
func (p panickingChatProvider) Chat(core.ProcessView) core.ChatCapability {
	panic(p.cause)
}

func TestChatProvider_RejectsStreamerWithoutModel(t *testing.T) {
	platformClient, _ := chatclient.New(newRecordingModel())
	a := chatAgent(t)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: platformClient, Streamer: platformClient}})
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	process, err := engine.Run(
		t.Context(),
		a,
		core.Input(callIn{V: 1}),
		core.ProcessOptions{Extensions: []core.Extension{streamingOnlyProvider{}}},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	failure := process.Failure()
	if failure == nil || !strings.Contains(failure.Error(), "Streamer without a Model") {
		t.Fatalf("process failure = %v", failure)
	}
}

func TestChatProvider_PanicFailsProcess(t *testing.T) {
	cause := errors.New("chat provider sentinel")
	platformClient, _ := chatclient.New(newRecordingModel())
	a := chatAgent(t)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: platformClient}})
	process, err := engine.Run(
		t.Context(), a, core.Input(callIn{V: 1}),
		core.ProcessOptions{Extensions: []core.Extension{panickingChatProvider{cause: cause}}},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), cause) ||
		!strings.Contains(process.Failure().Error(), `chat provider "panic-chat" panicked`) {
		t.Fatalf("status/failure = %s/%v, want attributed chat provider panic", process.Status(), process.Failure())
	}
}
