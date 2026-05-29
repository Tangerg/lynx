package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestAutoSnapshot_PersistsTerminalState verifies that with AutoSnapshot on
// and a ProcessStore configured, a completed run is persisted automatically
// (no explicit SaveProcess call) and is loadable afterward.
func TestAutoSnapshot_PersistsTerminalState(t *testing.T) {
	a := agent.New("snap").
		Actions(agent.NewAction("count",
			func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	store := core.NewInMemoryProcessStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{
		ProcessStore: store,
		AutoSnapshot: true,
	})
	mustDeploy(t, platform, a)

	proc, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s", proc.Status())
	}

	snap, err := store.Load(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("auto-snapshot not persisted: %v", err)
	}
	if snap.Status != core.StatusCompleted {
		t.Fatalf("snapshot status = %s, want completed", snap.Status)
	}
	if snap.AgentName != "snap" {
		t.Fatalf("snapshot agent = %q", snap.AgentName)
	}
}

// TestAutoSnapshot_DisabledByDefault confirms the historical behavior: with
// a store configured but AutoSnapshot off, nothing is persisted unless the
// host calls SaveProcess explicitly.
func TestAutoSnapshot_DisabledByDefault(t *testing.T) {
	a := agent.New("nosnap").
		Actions(agent.NewAction("count",
			func(_ context.Context, _ *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "counted"})).
		Build()

	store := core.NewInMemoryProcessStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{ProcessStore: store})
	mustDeploy(t, platform, a)

	proc, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: word{Text: "lynx"}},
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	if _, err := store.Load(context.Background(), proc.ID()); err == nil {
		t.Fatal("expected nothing persisted when AutoSnapshot is off")
	}
}
