package exec

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestShells_RunReadKill drives the background-command lifecycle end to end: a
// command's output is captured and read incrementally, completion is reported,
// and kill stops a still-running shell.
func TestShells_RunReadKill(t *testing.T) {
	shells := NewShells()
	t.Cleanup(shells.KillAll)

	// A quick command: capture output + completion.
	id := shells.Launch(context.Background(), "", "printf hello", 0)
	waitDone(t, shells, id)
	out, _ := mustShell(t, shells, id).Read()
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want hello", out)
	}
	done, info := mustShell(t, shells, id).Status()
	if !done || info != "exit 0" {
		t.Errorf("status = (%v, %q), want done exit 0", done, info)
	}
	// Second read returns only new output (none) — incremental.
	if out2, _ := mustShell(t, shells, id).Read(); out2 != "" {
		t.Errorf("second read = %q, want empty (incremental)", out2)
	}

	// A long-running command: kill it.
	longID := shells.Launch(context.Background(), "", "sleep 30", 0)
	running, ok := shells.Kill(longID)
	if !ok || !running {
		t.Fatalf("kill = (running=%v ok=%v), want a running shell stopped", running, ok)
	}
	waitDone(t, shells, longID)
	if running2, _ := shells.Kill(longID); running2 {
		t.Error("second kill should report not-running")
	}
}

// TestShells_TimeoutKills checks the hard-timeout path: a command outliving
// its timeout is killed, and Outcome reports it as killed with a duration.
func TestShells_TimeoutKills(t *testing.T) {
	shells := NewShells()
	t.Cleanup(shells.KillAll)

	id := shells.Launch(context.Background(), "", "sleep 30", 200*time.Millisecond)
	sh := mustShell(t, shells, id)
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

func waitDone(t *testing.T, shells *Shells, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sh, ok := shells.Get(id); ok {
			if done, _ := sh.Status(); done {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("shell %s did not finish in time", id)
}

func mustShell(t *testing.T, shells *Shells, id string) *Shell {
	t.Helper()
	sh, ok := shells.Get(id)
	if !ok {
		t.Fatalf("shell %s not found", id)
	}
	return sh
}
