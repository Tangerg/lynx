package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestBgShell_RunReadKill drives the background-command lifecycle end to end: a
// command's output is captured and read incrementally, completion is reported,
// and kill stops a still-running shell.
func TestBgShell_RunReadKill(t *testing.T) {
	mgr := newBgShellManager()
	t.Cleanup(mgr.killAll)

	// A quick command: capture output + completion.
	id := mgr.launch("", "printf hello")
	waitDone(t, mgr, id)
	out, _ := mustShell(t, mgr, id).read()
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q, want hello", out)
	}
	done, info := mustShell(t, mgr, id).status()
	if !done || info != "exit 0" {
		t.Errorf("status = (%v, %q), want done exit 0", done, info)
	}
	// Second read returns only new output (none) — incremental.
	if out2, _ := mustShell(t, mgr, id).read(); out2 != "" {
		t.Errorf("second read = %q, want empty (incremental)", out2)
	}

	// A long-running command: kill it.
	longID := mgr.launch("", "sleep 30")
	running, ok := mgr.kill(longID)
	if !ok || !running {
		t.Fatalf("kill = (running=%v ok=%v), want a running shell stopped", running, ok)
	}
	waitDone(t, mgr, longID)
	if running2, _ := mgr.kill(longID); running2 {
		t.Error("second kill should report not-running")
	}
}

// TestBgShell_Tools checks the model-facing tool surface: run returns a shell
// id, and output reports an unknown id gracefully (not an error).
func TestBgShell_Tools(t *testing.T) {
	mgr := newBgShellManager()
	t.Cleanup(mgr.killAll)
	tools := buildBgShellTools(mgr, t.TempDir())
	if len(tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(tools))
	}
	var run, output chat.Tool
	for _, tl := range tools {
		switch tl.Definition().Name {
		case "run_in_background":
			run = tl
		case "bash_output":
			output = tl
		}
	}
	out, err := run.Call(context.Background(), `{"command":"printf hi"}`)
	if err != nil || !strings.Contains(out, "Started background shell bg_1") {
		t.Fatalf("run = %q err=%v, want a started message", out, err)
	}
	miss, err := output.Call(context.Background(), `{"shell_id":"bg_999"}`)
	if err != nil || !strings.Contains(miss, "No background shell") {
		t.Fatalf("output(unknown) = %q err=%v", miss, err)
	}
}

func waitDone(t *testing.T, mgr *bgShellManager, id string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if sh, ok := mgr.get(id); ok {
			if done, _ := sh.status(); done {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("shell %s did not finish in time", id)
}

func mustShell(t *testing.T, mgr *bgShellManager, id string) *bgShell {
	t.Helper()
	sh, ok := mgr.get(id)
	if !ok {
		t.Fatalf("shell %s not found", id)
	}
	return sh
}
