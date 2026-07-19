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
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})

	// A quick command: capture output + completion.
	id, err := shells.Launch(context.Background(), "", "", "printf hello", 0)
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
	longID, err := shells.Launch(context.Background(), "", "", "sleep 30", 0)
	if err != nil {
		t.Fatal(err)
	}
	running, err := shells.Kill(longID)
	if err != nil || !running {
		t.Fatalf("kill = (running=%v err=%v), want a running shell stopped", running, err)
	}
	waitDone(t, shells, longID)
	if running2, err := shells.Kill(longID); err != nil || running2 {
		t.Error("second kill should report not-running")
	}
}

// TestShells_TimeoutKills checks the hard-timeout path: a command outliving
// its timeout is killed, and Outcome reports it as killed with a duration.
func TestShells_TimeoutKills(t *testing.T) {
	shells := NewShells()
	t.Cleanup(func() {
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
	})

	id, err := shells.Launch(context.Background(), "", "", "sleep 30", 200*time.Millisecond)
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
	id, err := shells.Launch(context.Background(), "", "", "sleep 30", 0)
	if err != nil {
		t.Fatal(err)
	}
	sh := mustShell(t, shells, id)

	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll: %v", err)
	}
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
	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll: %v", err)
	}
	if _, err := shells.Launch(context.Background(), "", "", "printf late", 0); !errors.Is(err, ErrShellsClosed) {
		t.Fatalf("Launch after KillAll = %v, want ErrShellsClosed", err)
	}
}

func TestShellsKillMissingHasStableIdentity(t *testing.T) {
	shells := NewShells()
	if _, err := shells.Kill("bg_missing"); !errors.Is(err, ErrShellNotFound) {
		t.Fatalf("Kill missing shell = %v, want ErrShellNotFound", err)
	}
}

func TestShellsFailedLaunchCanBeShutDown(t *testing.T) {
	shells := NewShells()
	id, err := shells.Launch(t.Context(), "", t.TempDir()+"/missing", "printf unreachable", 0)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if id == "" {
		t.Fatal("failed launch has no shell identity")
	}
	if err := shells.KillAll(); err != nil {
		t.Fatalf("KillAll after failed launch: %v", err)
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
			id, err := shells.Launch(context.Background(), "", "", "sleep 30", 0)
			result <- struct {
				id  string
				err error
			}{id: id, err: err}
		}()
		if err := shells.KillAll(); err != nil {
			t.Errorf("KillAll: %v", err)
		}
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

// TestShells_RunningForSession scopes the live-shell readout to one session and
// drops shells that have finished.
func TestShells_RunningForSession(t *testing.T) {
	shells := NewShells()
	t.Cleanup(func() { _ = shells.KillAll() })

	if _, err := shells.Launch(context.Background(), "sess-a", "", "sleep 30", 0); err != nil {
		t.Fatalf("launch a1: %v", err)
	}
	if _, err := shells.Launch(context.Background(), "sess-a", "", "sleep 30", 0); err != nil {
		t.Fatalf("launch a2: %v", err)
	}
	bID, err := shells.Launch(context.Background(), "sess-b", "", "sleep 30", 0)
	if err != nil {
		t.Fatalf("launch b: %v", err)
	}

	if got := shells.RunningForSession("sess-a"); len(got) != 2 {
		t.Fatalf("session a running = %d, want 2", len(got))
	}
	if got := shells.RunningForSession("sess-a")[0].Command; got != "sleep 30" {
		t.Fatalf("running shell command = %q, want %q", got, "sleep 30")
	}
	if got := shells.RunningForSession("other"); len(got) != 0 {
		t.Fatalf("unknown session running = %d, want 0", len(got))
	}

	// A killed shell drops out of its session's live set.
	if _, err := shells.Kill(bID); err != nil {
		t.Fatalf("kill b: %v", err)
	}
	waitForDone(t, shells, bID)
	if got := shells.RunningForSession("sess-b"); len(got) != 0 {
		t.Fatalf("session b after kill = %d, want 0", len(got))
	}
}

func waitForDone(t *testing.T, shells *Shells, id string) {
	t.Helper()
	sh, ok := shells.Get(id)
	if !ok {
		return
	}
	select {
	case <-sh.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("shell %q did not finish after kill", id)
	}
}
