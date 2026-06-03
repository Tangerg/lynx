package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestTypedActionAwaitInputSuspendsAndResumes proves a TYPED action
// (NewAction[In,Out], whose fn returns (Out, error)) can suspend on
// AwaitInput and resume to completion. Before the typed-action HITL
// support, the wrapper always returned Succeeded/Failed, so any HITL
// flow needed a raw untyped action; now the wrapper translates a parked
// awaitable into ActionWaiting.
//
// Flow: first run → no decision on the blackboard → AwaitInput →
// StatusWaiting. ResumeProcess(true) runs the handler (writes the
// decision), ContinueProcess re-runs the action, which now sees the
// decision and produces output → StatusCompleted.
func TestTypedActionAwaitInputSuspendsAndResumes(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	const approvedKey = "approved"
	gate := agent.New("typed-gate").
		Description("typed action that awaits a decision, then completes").
		Actions(agent.NewAction("gate",
			func(_ context.Context, pc *core.ProcessContext, _ subInput) (subOutput, error) {
				if v, ok := pc.Blackboard.Condition(approvedKey); ok {
					if !v {
						return subOutput{Doubled: -1}, nil // rejected
					}
					return subOutput{Doubled: 42}, nil // approved
				}
				req := hitl.NewConfirmation("approve?", func(approved bool) core.ResponseImpact {
					pc.Blackboard.SetCondition(approvedKey, approved)
					return core.ImpactUpdated
				})
				pc.AwaitInput(req)
				return subOutput{}, nil // suspends: typed wrapper sees InputAwaited
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[subOutput](core.Goal{Description: "gated output"})).
		Build()
	if err := platform.Deploy(gate); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	ctx := context.Background()
	proc, done := platform.StartAgent(ctx, gate,
		map[string]any{core.DefaultBindingName: subInput{Value: 1}}, core.ProcessOptions{})
	<-done
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("after start: status = %v, want waiting", proc.Status())
	}

	if _, err := platform.ResumeProcess(proc.ID(), true); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := platform.ContinueProcess(ctx, proc.ID()); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("after resume: status = %v, want completed", proc.Status())
	}
	out, ok := core.ResultOfType[subOutput](proc)
	if !ok || out.Doubled != 42 {
		t.Fatalf("result = %+v ok=%v, want Doubled=42", out, ok)
	}
}
