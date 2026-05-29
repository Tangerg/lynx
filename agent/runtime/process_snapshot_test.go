package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/runtime"
)

type ssWord struct{ Text string }
type ssWordCount struct{ Count int }

// buildSnapshotAgent constructs a single-action agent suitable for
// snapshot/restore exercises.
func buildSnapshotAgent() *core.Agent {
	return agent.New("snapshot-agent").
		Actions(agent.NewAction("count",
			func(ctx context.Context, pc *core.ProcessContext, in ssWord) (ssWordCount, error) {
				return ssWordCount{Count: len(in.Text)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[ssWordCount](core.Goal{Description: "word counted"})).
		Build()
}

func TestPlatform_SaveProcess_NoStore(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	a := buildSnapshotAgent()
	mustDeploy(t, platform, a)

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBindingName: ssWord{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := platform.SaveProcess(context.Background(), proc.ID()); err == nil {
		t.Error("expected error when no ProcessStore configured")
	}
}

func TestPlatform_SaveAndRestore_RoundTrip(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{
		ProcessStore: store,
	})
	a := buildSnapshotAgent()
	mustDeploy(t, platform, a)

	proc, err := platform.RunAgent(
		context.Background(), a,
		map[string]any{core.DefaultBindingName: ssWord{Text: "lynx"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}

	if err := platform.SaveProcess(context.Background(), proc.ID()); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify snapshot in store.
	snap, err := store.Load(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("store load: %v", err)
	}
	if snap.AgentName != "snapshot-agent" || snap.Status != core.StatusCompleted {
		t.Errorf("snapshot fields wrong: %#v", snap)
	}
	if len(snap.History) == 0 {
		t.Error("expected at least 1 history entry")
	}

	// Restore on a fresh platform with the same agent deployed.
	platform2 := agent.NewPlatform(runtime.PlatformConfig{ProcessStore: store})
	mustDeploy(t, platform2, buildSnapshotAgent())

	restored, err := platform2.RestoreProcess(context.Background(), proc.ID())
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.ID() != proc.ID() {
		t.Errorf("id: want %q, got %q", proc.ID(), restored.ID())
	}
	if restored.Status() != core.StatusCompleted {
		t.Errorf("status: want completed, got %s", restored.Status())
	}
	if len(restored.History()) != len(snap.History) {
		t.Errorf("history len mismatch: want %d, got %d", len(snap.History), len(restored.History()))
	}

	// The restored process's blackboard should still hold the word count.
	if _, ok := core.ResultOfType[ssWordCount](restored); !ok {
		t.Error("restored blackboard lost ssWordCount")
	}
}

// TestPlatform_RestoreWaitingProcess_ResumesToCompletion proves the
// full cross-restart HITL chain: a process parked on AwaitInput is
// snapshotted, restored on a FRESH platform (nothing shared but the
// store), re-continued to re-park its awaitable, resumed with a
// response, and driven to completion.
//
// The pending awaitable is deliberately NOT persisted — it carries an
// un-serializable handler closure (see ProcessSnapshot's package note).
// So the contract is: a restored Waiting process must re-tick once to
// let the awaiting action re-issue AwaitInput against the restored
// blackboard. The action stays idempotent by reading its decision from
// a blackboard condition — undecided → re-park, decided → proceed.
//
// This exercises a restored process's tick loop, which the terminal-
// state round-trip tests never do.
func TestPlatform_RestoreWaitingProcess_ResumesToCompletion(t *testing.T) {
	const approvedKey = "approved"
	buildGate := func() *core.Agent {
		return agent.New("waiting-gate").
			Actions(agent.NewAction("gate",
				func(_ context.Context, pc *core.ProcessContext, _ ssWord) (ssWordCount, error) {
					if v, ok := pc.Blackboard.Condition(approvedKey); ok {
						if !v {
							return ssWordCount{Count: -1}, nil // rejected
						}
						return ssWordCount{Count: 42}, nil // approved
					}
					pc.AwaitInput(hitl.NewConfirmation("approve?", func(approved bool) core.ResponseImpact {
						pc.Blackboard.SetCondition(approvedKey, approved)
						return core.ImpactUpdated
					}))
					return ssWordCount{}, nil // suspends: wrapper sees InputAwaited
				},
				core.ActionConfig{},
			)).
			Goals(agent.GoalProducing[ssWordCount](core.Goal{Description: "gated output"})).
			Build()
	}

	store := core.NewInMemoryProcessStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{ProcessStore: store})
	mustDeploy(t, platform, buildGate())

	ctx := context.Background()
	proc, done := platform.StartAgent(ctx, buildGate(),
		map[string]any{core.DefaultBindingName: ssWord{Text: "hi"}}, core.ProcessOptions{})
	<-done
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("after start: status = %v, want waiting", proc.Status())
	}

	// Persist the WAITING process, then walk away from the original
	// platform entirely.
	if err := platform.SaveProcess(ctx, proc.ID()); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Fresh platform — shares only the store.
	platform2 := agent.NewPlatform(runtime.PlatformConfig{ProcessStore: store})
	mustDeploy(t, platform2, buildGate())

	restored, err := platform2.RestoreProcess(ctx, proc.ID())
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status() != core.StatusWaiting {
		t.Fatalf("restored status = %v, want waiting", restored.Status())
	}
	// The awaitable's closure can't round-trip, so nothing is parked yet.
	if restored.PendingAwaitable() != nil {
		t.Fatal("freshly-restored process should have no parked awaitable")
	}

	// Re-tick: the gate action re-issues AwaitInput against the restored
	// blackboard (decision still unset → re-park → back to Waiting).
	if err := platform2.ContinueProcess(ctx, restored.ID()); err != nil {
		t.Fatalf("re-continue: %v", err)
	}
	if restored.Status() != core.StatusWaiting {
		t.Fatalf("after re-continue: status = %v, want waiting", restored.Status())
	}
	if restored.PendingAwaitable() == nil {
		t.Fatal("awaitable should be re-parked after re-continue")
	}

	// Resume with approval, then drive to completion.
	if _, err := platform2.ResumeProcess(restored.ID(), true); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := platform2.ContinueProcess(ctx, restored.ID()); err != nil {
		t.Fatalf("continue: %v", err)
	}
	if restored.Status() != core.StatusCompleted {
		t.Fatalf("after resume: status = %v, want completed; failure=%v", restored.Status(), restored.Failure())
	}
	out, ok := core.ResultOfType[ssWordCount](restored)
	if !ok || out.Count != 42 {
		t.Fatalf("result = %+v ok=%v, want Count=42", out, ok)
	}
}

func TestPlatform_RestoreProcess_AgentNotDeployed(t *testing.T) {
	store := core.NewInMemoryProcessStore()
	platform := agent.NewPlatform(runtime.PlatformConfig{ProcessStore: store})

	_ = store.Save(context.Background(), core.ProcessSnapshot{
		ID:        "orphan",
		AgentName: "never-deployed",
		Status:    core.StatusCompleted,
	})

	if _, err := platform.RestoreProcess(context.Background(), "orphan"); err == nil {
		t.Error("expected error when agent not deployed")
	}
}
