package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestKillProcess_IdempotentNoClobber pins that KillProcess never clobbers a
// terminal process: killing a Completed process must leave it Completed (a kill
// racing a natural completion must not rewrite the outcome to Killed), and a
// repeat kill is a no-op. The check-and-set is atomic (markKilled), so the
// primitive is safe for any caller — not just KillChildren, which used to be
// the only guarded path. (buildSnapshotAgent/ssWord live in
// process_snapshot_test.go, mustDeploy in deploy_support_test.go.)
func TestKillProcess_IdempotentNoClobber(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, buildSnapshotAgent())

	proc, err := platform.RunAgent(context.Background(), buildSnapshotAgent(),
		map[string]any{core.DefaultBindingName: ssWord{Text: "x"}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s, want completed; failure=%v", proc.Status(), proc.Failure())
	}

	// Kill a completed process — must NOT clobber Completed -> Killed.
	if err := platform.KillProcess(proc.ID()); err != nil {
		t.Fatalf("KillProcess: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Errorf("after KillProcess: status = %s, want completed — a kill must not clobber a terminal process", proc.Status())
	}

	// Repeat kill is a no-op (still completed, no error).
	if err := platform.KillProcess(proc.ID()); err != nil {
		t.Fatalf("second KillProcess: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Errorf("after second KillProcess: status = %s, want completed", proc.Status())
	}
}
