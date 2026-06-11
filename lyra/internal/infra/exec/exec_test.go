package exec

import (
	"strings"
	"testing"
	"time"
)

// TestManager_RunReadKill drives the background-command lifecycle end to end: a
// command's output is captured and read incrementally, completion is reported,
// and kill stops a still-running shell.
func TestManager_RunReadKill(t *testing.T) {
	mgr := NewManager()
	t.Cleanup(mgr.KillAll)

	// A quick command: capture output + completion.
	id := mgr.Launch("", "printf hello")
	waitDone(t, mgr, id)
	out, _ := mustShell(t, mgr, id).Read()
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want hello", out)
	}
	done, info := mustShell(t, mgr, id).Status()
	if !done || info != "exit 0" {
		t.Errorf("status = (%v, %q), want done exit 0", done, info)
	}
	// Second read returns only new output (none) — incremental.
	if out2, _ := mustShell(t, mgr, id).Read(); out2 != "" {
		t.Errorf("second read = %q, want empty (incremental)", out2)
	}

	// A long-running command: kill it.
	longID := mgr.Launch("", "sleep 30")
	running, ok := mgr.Kill(longID)
	if !ok || !running {
		t.Fatalf("kill = (running=%v ok=%v), want a running shell stopped", running, ok)
	}
	waitDone(t, mgr, longID)
	if running2, _ := mgr.Kill(longID); running2 {
		t.Error("second kill should report not-running")
	}
}

func waitDone(t *testing.T, mgr *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sh, ok := mgr.Get(id); ok {
			if done, _ := sh.Status(); done {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("shell %s did not finish in time", id)
}

func mustShell(t *testing.T, mgr *Manager, id string) *Shell {
	t.Helper()
	sh, ok := mgr.Get(id)
	if !ok {
		t.Fatalf("shell %s not found", id)
	}
	return sh
}
