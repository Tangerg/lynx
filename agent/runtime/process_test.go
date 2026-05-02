package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type word struct{ Text string }
type wordCount struct{ Count int }

// TestRunSingleAction verifies the smallest end-to-end loop: one input, one
// action, one goal. Ensures the planner finds the (single) action and the
// runtime executes it to completion.
func TestRunSingleAction(t *testing.T) {
	a := agent.New(core.AgentMeta{Name: "test"}).
		Actions(agent.NewAction("count",
			func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
				return wordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[wordCount](core.Goal{Description: "word counted"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBinding: word{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[wordCount](proc)
	if !ok {
		t.Fatal("no wordCount produced")
	}
	if got.Count != 4 {
		t.Fatalf("count: got %d want 4", got.Count)
	}
}

// TestRunMultiStepPlanning confirms the GOAP planner sequences three actions
// correctly: A produces X, B consumes X to produce Y, C consumes Y to produce
// the goal type.
func TestRunMultiStepPlanning(t *testing.T) {
	type stage1 struct{ V int }
	type stage2 struct{ V int }
	type stage3 struct{ V int }

	a := agent.New(core.AgentMeta{Name: "multi"}).
		Actions(agent.NewAction("a",
			func(ctx context.Context, pc *core.ProcessContext, in word) (stage1, error) {
				return stage1{V: len(in.Text)}, nil
			}, core.ActionConfig{})).
		Actions(agent.NewAction("b",
			func(ctx context.Context, pc *core.ProcessContext, in stage1) (stage2, error) {
				return stage2{V: in.V * 2}, nil
			}, core.ActionConfig{})).
		Actions(agent.NewAction("c",
			func(ctx context.Context, pc *core.ProcessContext, in stage2) (stage3, error) {
				return stage3{V: in.V + 1}, nil
			}, core.ActionConfig{})).
		Goals(agent.GoalProducing[stage3](core.Goal{Description: "stage3 produced"})).
		Build()

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatal(err)
	}

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBinding: word{Text: "abcd"}}, // len=4 → 4*2+1=9
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	got, ok := core.ResultOfType[stage3](proc)
	if !ok {
		t.Fatalf("no stage3; status=%s", proc.Status())
	}
	if got.V != 9 {
		t.Fatalf("got %d want 9", got.V)
	}
	// Three actions, three ticks.
	if len(proc.History()) != 3 {
		t.Fatalf("history length %d, want 3", len(proc.History()))
	}
}
