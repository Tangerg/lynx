package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/infra/exec"
)

// TestBgShell_Tools checks the model-facing tool surface built over the
// background-process manager: run returns a shell id, and output reports an
// unknown id gracefully (not an error). The process mechanism itself is tested
// in internal/infra/exec.
func TestBgShell_Tools(t *testing.T) {
	mgr := exec.NewManager()
	t.Cleanup(mgr.KillAll)
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
