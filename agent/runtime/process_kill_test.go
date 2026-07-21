package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

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

	proc, err := engine.Run(context.Background(), buildSnapshotAgent(),
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

func TestKillTerminalParentStillKillsLiveDescendants(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	release := make(chan struct{})
	childAgent := blockingChild("terminal-parent-child", release)
	childDeployment, err := engine.Deploy(t.Context(), childAgent)
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	var child *runtime.Process
	var childDone <-chan error
	parentAgent := agent.New(agent.AgentConfig{
		Name: "terminal-parent",
		Actions: []agent.Action{agent.NewAction("start", func(ctx context.Context, _ *core.ProcessContext, in subInput) (parentOutput, error) {
			child, childDone, err = engine.StartChild(ctx, childDeployment, in)
			return parentOutput{Final: 1}, err
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "spawned"})},
	})
	mustDeploy(t, engine, parentAgent)
	parent, err := engine.Run(t.Context(), parentAgent, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run parent: %v", err)
	}
	if parent.Status() != core.StatusCompleted || child == nil || child.Status().IsTerminal() {
		t.Fatalf("parent/child status = %s/%v", parent.Status(), child)
	}

	if err := engine.Kill(t.Context(), parent.ID()); err != nil {
		t.Fatalf("Kill terminal parent: %v", err)
	}
	if parent.Status() != core.StatusCompleted {
		t.Fatalf("parent status = %s, want completed", parent.Status())
	}
	if child.Status() != core.StatusKilled {
		t.Fatalf("child status = %s, want killed", child.Status())
	}
	<-childDone
	close(release)
}

func TestRemoveRejectsActiveProcess(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	release := make(chan struct{})
	a := blockingChild("remove-active", release)
	mustDeploy(t, engine, a)

	proc, done, err := engine.Start(t.Context(), a, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := engine.Remove(proc.ID()); !errors.Is(err, runtime.ErrProcessActive) {
		t.Fatalf("Remove active process error = %v, want ErrProcessActive", err)
	}
	if _, ok := engine.Process(proc.ID()); !ok {
		t.Fatal("active process was removed")
	}
	if err := engine.Kill(t.Context(), proc.ID()); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	<-done
	close(release)
	if err := engine.Remove(proc.ID()); err != nil {
		t.Fatalf("Remove terminal process: %v", err)
	}
	if _, ok := engine.Process(proc.ID()); ok {
		t.Fatal("terminal process remains registered")
	}
}
