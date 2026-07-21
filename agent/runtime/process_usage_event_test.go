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

// modelCallCapture records model and embedding call events.
type modelCallCapture struct {
	mu             sync.Mutex
	modelCalls     []core.ModelCall
	embeddingCalls []core.EmbeddingCall
}

func (*modelCallCapture) Name() string { return "model-call-capture" }

func (c *modelCallCapture) OnEvent(_ context.Context, e event.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch ev := e.(type) {
	case event.ModelCallRecorded:
		c.modelCalls = append(c.modelCalls, ev.Call)
	case event.EmbeddingCallRecorded:
		c.embeddingCalls = append(c.embeddingCalls, ev.Call)
	}
}

// TestModelCallsPublishEvents verifies that recording model and embedding
// calls from inside an action surfaces on the event stream
// (with the timestamp defaulted), not just on Process.ModelCalls().
func TestModelCallsPublishEvents(t *testing.T) {
	capture := &modelCallCapture{}

	definition := agent.New(agent.AgentConfig{Name: "usage", Actions: []agent.Action{agent.NewAction("spend", func(ctx context.Context, process *core.ProcessContext, input word) (wordCount, error) {
		if err := process.RecordModelCall(ctx, core.ModelCall{Model: "claude-x", Provider: "anthropic", CostUSD: 0.01, PromptTokens: 100, CompletionTokens: 20}); err != nil {
			return wordCount{}, err
		}
		if err := process.RecordEmbeddingCall(ctx, core.EmbeddingCall{Model: "embed-x", CostUSD: 0.001, InputTokens: 50, InputCount: 2}); err != nil {
			return wordCount{}, err
		}
		return wordCount{Count: len(input.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{capture},
	})
	mustDeploy(t, engine, definition)

	if _, err := engine.Run(context.Background(), definition,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.modelCalls) != 1 {
		t.Fatalf("model call events = %d, want 1", len(capture.modelCalls))
	}
	if capture.modelCalls[0].Model != "claude-x" || capture.modelCalls[0].CostUSD != 0.01 {
		t.Fatalf("model call event payload: %#v", capture.modelCalls[0])
	}
	if capture.modelCalls[0].Timestamp.IsZero() {
		t.Error("expected model call timestamp to be defaulted")
	}
	if len(capture.embeddingCalls) != 1 {
		t.Fatalf("embedding call events = %d, want 1", len(capture.embeddingCalls))
	}
	if capture.embeddingCalls[0].Model != "embed-x" || capture.embeddingCalls[0].InputCount != 2 {
		t.Fatalf("embedding event payload: %#v", capture.embeddingCalls[0])
	}
}
