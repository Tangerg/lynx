package runtime_test

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/model/chat"
)

// recordingModel is a chat.Model that flips a flag when it's called, so a
// test can tell which client a turn actually routed through.
type recordingModel struct {
	defaults *chat.Options
	called   bool
}

func newRecordingModel() *recordingModel {
	opts, _ := chat.NewOptions("stub-model")
	return &recordingModel{defaults: opts}
}

func (m *recordingModel) DefaultOptions() chat.Options { return *m.defaults }
func (m *recordingModel) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *recordingModel) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	m.called = true
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage("ok"),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp, nil
}

func (m *recordingModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// fixedClientProvider is a ChatClientProvider that always hands back one
// preset client — the per-process override shape (lyra registers one of
// these per turn carrying the run's chosen model).
type fixedClientProvider struct {
	name   string
	client *chat.Client
}

func (f fixedClientProvider) Name() string                            { return f.name }
func (f fixedClientProvider) ChatClientFor(core.Process) *chat.Client { return f.client }

var _ core.ChatClientProvider = fixedClientProvider{}

type callIn struct{ V int }
type callOut struct{ V int }

// callsChat is an action body that issues one LLM call through whichever
// client the ProcessContext resolved — the observable for which client won.
func callsChat(t *testing.T) func(context.Context, *core.ProcessContext, callIn) (callOut, error) {
	return func(ctx context.Context, pc *core.ProcessContext, in callIn) (callOut, error) {
		req := pc.Chat()
		if req == nil {
			t.Error("pc.Chat() returned nil — no client resolved")
			return callOut{}, nil
		}
		if _, err := req.WithUserPrompt("hi").Call().Response(ctx); err != nil {
			t.Errorf("chat call: %v", err)
		}
		return callOut{V: in.V + 1}, nil
	}
}

func chatAgent(t *testing.T) *core.Agent {
	return agent.New("chat-router").
		Actions(agent.NewAction("call", callsChat(t), core.ActionConfig{})).
		Goals(agent.GoalProducing[callOut](core.Goal{Description: "done"})).
		Build()
}

// TestChatClientProvider_OverridesPlatformClient verifies a per-process
// ChatClientProvider extension wins over the platform's shared client — the
// mechanism that lets one Platform serve turns against different models.
func TestChatClientProvider_OverridesPlatformClient(t *testing.T) {
	platformModel := newRecordingModel()
	overrideModel := newRecordingModel()
	platformClient, _ := chat.NewClient(platformModel)
	overrideClient, _ := chat.NewClient(overrideModel)

	a := chatAgent(t)
	platform := agent.NewPlatform(runtime.PlatformConfig{ChatClient: platformClient})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	_, err := platform.RunAgent(
		t.Context(), a,
		map[string]any{core.DefaultBindingName: callIn{V: 1}},
		core.ProcessOptions{
			Extensions: []core.Extension{
				fixedClientProvider{name: "per-run-model", client: overrideClient},
			},
		},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if !overrideModel.called {
		t.Error("override client was not used")
	}
	if platformModel.called {
		t.Error("platform client was used despite a ChatClientProvider override")
	}
}

// TestChatClientProvider_FallsBackToPlatform verifies that with no provider
// registered (or one that returns nil), the platform's client is used.
func TestChatClientProvider_FallsBackToPlatform(t *testing.T) {
	platformModel := newRecordingModel()
	platformClient, _ := chat.NewClient(platformModel)

	a := chatAgent(t)
	platform := agent.NewPlatform(runtime.PlatformConfig{ChatClient: platformClient})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// A provider that defers (nil) must not shadow the platform client.
	_, err := platform.RunAgent(
		t.Context(), a,
		map[string]any{core.DefaultBindingName: callIn{V: 1}},
		core.ProcessOptions{
			Extensions: []core.Extension{
				fixedClientProvider{name: "defers", client: nil},
			},
		},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if !platformModel.called {
		t.Error("platform client was not used as the fallback")
	}
}
