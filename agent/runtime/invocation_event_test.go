package runtime_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

// invocationCapture is an EventListener extension that records the
// per-call invocation events the runtime now publishes.
type invocationCapture struct {
	mu  sync.Mutex
	llm []core.LLMInvocation
	emb []core.EmbeddingInvocation
}

func (*invocationCapture) Name() string { return "invocation-capture" }

func (c *invocationCapture) OnEvent(_ context.Context, e event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch ev := e.(type) {
	case event.LLMInvocationRecorded:
		c.llm = append(c.llm, ev.Invocation)
	case event.EmbeddingInvocationRecorded:
		c.emb = append(c.emb, ev.Invocation)
	}
}

// TestRecordInvocation_PublishesEvents verifies that recording an LLM /
// embedding invocation from inside an action surfaces on the event stream
// (with the timestamp defaulted), not just on Process.LLMInvocations().
func TestRecordInvocation_PublishesEvents(t *testing.T) {
	capture := &invocationCapture{}

	a := agent.New("usage").
		Actions(agent.NewAction("spend",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				pc.RecordLLMInvocation(ctx, core.LLMInvocation{
					Model: "claude-x", Provider: "anthropic",
					CostUSD: 0.01, PromptTokens: 100, CompletionTokens: 20,
				})
				pc.RecordEmbeddingInvocation(ctx, core.EmbeddingInvocation{
					Model: "embed-x", CostUSD: 0.001, InputTokens: 50, InputCount: 2,
				})
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "done"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{
		Extensions: []core.Extension{capture},
	})
	mustDeploy(t, platform, a)

	if _, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{}); err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.llm) != 1 {
		t.Fatalf("LLM invocation events = %d, want 1", len(capture.llm))
	}
	if capture.llm[0].Model != "claude-x" || capture.llm[0].CostUSD != 0.01 {
		t.Fatalf("llm event payload: %#v", capture.llm[0])
	}
	if capture.llm[0].Timestamp.IsZero() {
		t.Error("expected LLM invocation timestamp to be defaulted")
	}
	if len(capture.emb) != 1 {
		t.Fatalf("embedding invocation events = %d, want 1", len(capture.emb))
	}
	if capture.emb[0].Model != "embed-x" || capture.emb[0].InputCount != 2 {
		t.Fatalf("embedding event payload: %#v", capture.emb[0])
	}
}
