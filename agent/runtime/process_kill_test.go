package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

func blockingChild(name string, release <-chan struct{}) *core.Agent {
	return agent.New(agent.Config{
		Name: name,
		Actions: []agent.Action{agent.NewAction(
			"work",
			func(ctx context.Context, _ *core.ProcessContext, in subInput) (subOutput, error) {
				select {
				case <-release:
					return subOutput{Doubled: in.Value * 2}, nil
				case <-ctx.Done():
					return subOutput{}, ctx.Err()
				}
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[subOutput](core.GoalConfig{Description: "doubled"})},
	})
}

// TestKillProcess_IdempotentNoClobber pins that Kill never clobbers a
// terminal process: killing a Completed process must leave it Completed (a kill
// racing a natural completion must not rewrite the outcome to Killed), and a
// repeat kill is a no-op. The check-and-set is atomic (markKilled), so the
// primitive is safe for any caller — not just KillChildren, which used to be
// the only guarded path. (buildSnapshotAgent/ssWord live in
// process_snapshot_test.go, mustDeploy in deploy_support_test.go.)
func TestKillProcess_IdempotentNoClobber(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, buildSnapshotAgent())

	proc, err := engine.Run(t.Context(), buildSnapshotAgent(),
		core.Input(ssWord{Text: "x"}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s, want completed; failure=%v", proc.Status(), proc.Failure())
	}

	// Kill a completed process — must NOT clobber Completed -> Killed.
	if err := engine.Kill(t.Context(), proc.ID()); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Errorf("after Kill: status = %s, want completed — a kill must not clobber a terminal process", proc.Status())
	}

	// Repeat kill is a no-op (still completed, no error).
	if err := engine.Kill(t.Context(), proc.ID()); err != nil {
		t.Fatalf("second Kill: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Errorf("after second Kill: status = %s, want completed", proc.Status())
	}
}

func TestKillListenerMayReenterSameProcessTree(t *testing.T) {
	var engine *runtime.Engine
	reentered := make(chan error, 1)
	listener := runtime.NewEventListener("reentrant-kill", func(ctx context.Context, published event.Event) {
		killed, ok := published.(event.ProcessKilled)
		if !ok {
			return
		}
		reentered <- engine.Kill(ctx, killed.ProcessID())
	})
	engine = agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{listener}})
	release := make(chan struct{})
	definition := blockingChild("reentrant-kill", release)
	mustDeploy(t, engine, definition)
	runHandle, err := engine.Start(t.Context(), definition, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}

	killDone := make(chan error, 1)
	go func() { killDone <- engine.Kill(t.Context(), runHandle.Process().ID()) }()
	select {
	case err := <-reentered:
		if err != nil {
			t.Fatalf("reentrant Kill: %v", err)
		}
	case <-t.Context().Done():
		t.Fatal("ProcessKilled listener deadlocked on the process-tree mutation")
	}
	if err := <-killDone; err != nil {
		t.Fatalf("outer Kill: %v", err)
	}
	awaitRun(t, runHandle)
	close(release)
}

func TestRemoveTreeRejectsActiveProcess(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	release := make(chan struct{})
	a := blockingChild("remove-active", release)
	mustDeploy(t, engine, a)

	runHandle, err := engine.Start(t.Context(), a, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	proc := runHandle.Process()
	if err := engine.RemoveTree(t.Context(), proc.ID()); !errors.Is(err, runtime.ErrProcessActive) {
		t.Fatalf("RemoveTree active process error = %v, want ErrProcessActive", err)
	}
	if _, ok := engine.Process(proc.ID()); !ok {
		t.Fatal("active process was removed")
	}
	if err := engine.Kill(t.Context(), proc.ID()); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	awaitRun(t, runHandle)
	close(release)
	if err := engine.RemoveTree(t.Context(), proc.ID()); err != nil {
		t.Fatalf("RemoveTree terminal process: %v", err)
	}
	if _, ok := engine.Process(proc.ID()); ok {
		t.Fatal("terminal process remains registered")
	}
}
