package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
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
