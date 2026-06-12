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
	id := mgr.Launch("", "printf hello", 0)
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
	longID := mgr.Launch("", "sleep 30", 0)
	running, ok := mgr.Kill(longID)
	if !ok || !running {
		t.Fatalf("kill = (running=%v ok=%v), want a running shell stopped", running, ok)
	}
	waitDone(t, mgr, longID)
	if running2, _ := mgr.Kill(longID); running2 {
		t.Error("second kill should report not-running")
	}
}

// TestManager_TimeoutKills checks the hard-timeout path: a command outliving
// its timeout is killed, and Outcome reports it as killed with a duration.
func TestManager_TimeoutKills(t *testing.T) {
	mgr := NewManager()
	t.Cleanup(mgr.KillAll)

	id := mgr.Launch("", "sleep 30", 200*time.Millisecond)
	sh := mustShell(t, mgr, id)
	select {
	case <-sh.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("timed-out command did not finish")
	}
	_, killed, dur := sh.Outcome()
	if !killed {
		t.Error("Outcome.killed = false, want true (terminated by timeout)")
	}
	if dur <= 0 {
		t.Errorf("Outcome.duration = %v, want positive", dur)
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
