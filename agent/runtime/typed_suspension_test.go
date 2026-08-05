package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestTypedActionSuspendsAndResumes proves a typed action
// (NewAction[In,Out], whose fn returns (Out, error)) can park a snapshot-compatible
// Suspension and resume to completion.
//
// Flow: first run → Interrupt → StatusWaiting. Resume validates and
// records the response; Continue re-runs the action, which decodes it
// at the same call site and produces output → StatusCompleted.
func TestTypedActionSuspendsAndResumes(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	gate := agent.New(agent.Config{Name: "typed-gate", Description: "typed action that awaits a decision, then completes", Actions: []agent.Action{agent.NewAction("gate", func(ctx context.Context, _ *core.ProcessContext, _ subInput) (subOutput, error) {
		approved, err := hitl.Interrupt[bool](ctx, "approval", "approve?")
		if err != nil {
			return subOutput{}, err
		}
		if !approved {
			return subOutput{Doubled: -1}, nil
		}
		return subOutput{Doubled: 42}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "gated output"})}})
	if _, err := engine.Deploy(t.Context(), gate); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	ctx := t.Context()
	runHandle, err := engine.Start(ctx, gate,
		core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	completion := awaitRun(t, runHandle)
	if err := completion.Error(); err != nil {
		t.Fatalf("start run handle: %v", err)
	}
	proc := runHandle.Process()
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("after start: status = %v, want waiting", proc.Status())
	}
	if err := engine.Continue(ctx, proc.ID()); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("continue before response error = %v", err)
	}
	if err := engine.Respond(t.Context(), proc.ID(), "stale", true); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("stale resume error = %v", err)
	}
	if err := engine.Respond(t.Context(), proc.ID(), "approval", "yes"); err == nil {
		t.Fatal("schema-invalid response unexpectedly succeeded")
	}

	if err := engine.Respond(t.Context(), proc.ID(), "approval", true); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := engine.Respond(t.Context(), proc.ID(), "approval", true); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("duplicate resume error = %v", err)
	}
	if err := engine.Respond(t.Context(), proc.ID(), "approval", false); !errors.Is(err, interaction.ErrSuspensionStale) {
		t.Fatalf("conflicting resume error = %v", err)
	}
	if err := engine.Continue(ctx, proc.ID()); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("after resume: status = %v, want completed", proc.Status())
	}
	out, ok := core.Result[subOutput](proc)
	if !ok || out.Doubled != 42 {
		t.Fatalf("result = %+v ok=%v, want Doubled=42", out, ok)
	}
}
