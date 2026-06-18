package shell

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/exec"
)

// shellTool returns the named tool from a freshly-built shell tool set.
func shellTool(t *testing.T, mgr *exec.Manager, name string) chat.Tool {
	t.Helper()
	for _, tl := range Build(mgr, t.TempDir()) {
		if tl.Definition().Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not built", name)
	return nil
}

// TestBash_CompletesInline checks the foreground fast path: a quick command
// finishes within the auto-background window and returns its output + exit code
// inline (not as a background job).
func TestBash_CompletesInline(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	bash := shellTool(t, mgr, "bash")

	out, err := bash.Call(context.Background(), `{"command":"printf hello"}`)
	if err != nil {
		t.Fatalf("bash err = %v", err)
	}
	var res struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if json.Unmarshal([]byte(out), &res) != nil || res.Stdout != "hello" || res.ExitCode != 0 {
		t.Fatalf("result = %q, want {stdout:hello, exit_code:0}", out)
	}
	// A completed command is removed, not left as a background job.
	if _, ok := mgr.Get("bg_1"); ok {
		t.Error("finished command should be removed from the manager")
	}
}

// TestBash_RunInBackground checks the explicit-background path: the command
// returns a shell id immediately, and bash_output reads its output.
func TestBash_RunInBackground(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	bash := shellTool(t, mgr, "bash")
	output := shellTool(t, mgr, "bash_output")

	out, err := bash.Call(context.Background(), `{"command":"printf hi","run_in_background":true}`)
	if err != nil || !strings.Contains(out, "shell bg_1") {
		t.Fatalf("bash(bg) = %q err=%v, want a background notice with bg_1", out, err)
	}
	// No exit_code while it's a live job.
	if strings.Contains(out, "exit_code") {
		t.Errorf("backgrounded result must omit exit_code: %q", out)
	}
	sh, ok := mgr.Get("bg_1")
	if !ok {
		t.Fatal("background shell bg_1 should still be registered")
	}
	<-sh.Done()
	read, err := output.Call(context.Background(), `{"shell_id":"bg_1"}`)
	if err != nil || !strings.Contains(read, "hi") {
		t.Fatalf("bash_output = %q err=%v, want the command's output", read, err)
	}
}

// TestBashOutput_Wait blocks until a backgrounded command finishes, then
// returns its output + a finished status in a single call (the crush wait
// design — event-driven, no sleep poll loop).
func TestBashOutput_Wait(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	bash := shellTool(t, mgr, "bash")
	output := shellTool(t, mgr, "bash_output")

	out, err := bash.Call(context.Background(), `{"command":"sleep 0.3; printf done","run_in_background":true}`)
	if err != nil || !strings.Contains(out, "shell bg_1") {
		t.Fatalf("bash(bg) = %q err=%v", out, err)
	}
	// Without blocking it's still running; with block it waits to completion.
	read, err := output.Call(context.Background(), `{"shell_id":"bg_1","block":true}`)
	if err != nil {
		t.Fatalf("bash_output(block) err=%v", err)
	}
	if !strings.Contains(read, "done") || !strings.Contains(read, "finished") {
		t.Fatalf("bash_output(block) = %q, want finished output containing 'done'", read)
	}
}

// TestBashOutput_WaitTimeout returns the current still-running output (not an
// error) when timeout_seconds elapses before the command exits.
func TestBashOutput_WaitTimeout(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	bash := shellTool(t, mgr, "bash")
	output := shellTool(t, mgr, "bash_output")

	if _, err := bash.Call(context.Background(), `{"command":"sleep 30","run_in_background":true}`); err != nil {
		t.Fatalf("bash(bg) err=%v", err)
	}
	read, err := output.Call(context.Background(), `{"shell_id":"bg_1","block":true,"timeout":1000}`)
	if err != nil {
		t.Fatalf("bash_output(block,timeout) err=%v, want graceful still-running", err)
	}
	if !strings.Contains(read, "still running") {
		t.Fatalf("bash_output(block,timeout) = %q, want a still-running status", read)
	}
}

// TestBash_AutoBackground checks the promotion path: a command still running
// after auto_background_after seconds is moved to the background and stays
// addressable by its shell id.
func TestBash_AutoBackground(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	bash := shellTool(t, mgr, "bash")

	out, err := bash.Call(context.Background(), `{"command":"sleep 30","auto_background_after":1}`)
	if err != nil || !strings.Contains(out, "shell bg_1") {
		t.Fatalf("bash(auto-bg) = %q err=%v, want a background notice with bg_1", out, err)
	}
	if running, ok := mgr.Kill("bg_1"); !ok || !running {
		t.Fatalf("kill = (running=%v ok=%v), want the backgrounded shell still running", running, ok)
	}
}

// TestBashOutput_UnknownShell reports an unknown id gracefully (not an error).
func TestBashOutput_UnknownShell(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
	output := shellTool(t, mgr, "bash_output")

	miss, err := output.Call(context.Background(), `{"shell_id":"bg_999"}`)
	if err != nil || !strings.Contains(miss, "No background shell") {
		t.Fatalf("bash_output(unknown) = %q err=%v", miss, err)
	}
}
