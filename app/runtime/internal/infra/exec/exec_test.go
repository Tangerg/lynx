package exec

import (
	"context"
	"errors"
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
	id, err := shells.Launch(context.Background(), "", "printf hello", 0)
	if err != nil {
		t.Fatal(err)
	}
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
	longID, err := shells.Launch(context.Background(), "", "sleep 30", 0)
	if err != nil {
		t.Fatal(err)
	}
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

	id, err := shells.Launch(context.Background(), "", "sleep 30", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
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

func TestShellsKillAllJoinsProcesses(t *testing.T) {
	shells := NewShells()
	id, err := shells.Launch(context.Background(), "", "sleep 30", 0)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, id)

	shells.KillAll()
	select {
	case <-sh.Done():
	default:
		t.Fatal("KillAll returned before the process wait goroutine finished")
	}
	if _, ok := shells.Get(id); ok {
		t.Fatal("KillAll retained a stopped shell")
	}
}

func TestShellsRejectLaunchAfterKillAll(t *testing.T) {
	shells := NewShells()
	shells.KillAll()
	if _, err := shells.Launch(context.Background(), "", "printf late", 0); !errors.Is(err, ErrShellsClosed) {
		t.Fatalf("Launch after KillAll = %v, want ErrShellsClosed", err)
	}
}

func TestShellsLaunchRacesKillAll(t *testing.T) {
	for range 25 {
		shells := NewShells()
		result := make(chan struct {
			id  string
			err error
		}, 1)
		go func() {
			id, err := shells.Launch(context.Background(), "", "sleep 30", 0)
			result <- struct {
				id  string
				err error
			}{id: id, err: err}
		}()
		shells.KillAll()
		got := <-result
		if got.err != nil && !errors.Is(got.err, ErrShellsClosed) {
			t.Fatalf("Launch error = %v", got.err)
		}
		if got.id != "" {
			if _, ok := shells.Get(got.id); ok {
				t.Fatalf("KillAll retained racing shell %q", got.id)
			}
		}
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
