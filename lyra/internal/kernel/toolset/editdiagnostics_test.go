package toolset

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/codeintel"
)

// TestWithEditDiagnostics_AppendsProblems verifies the highest-value LSP
// integration: a successful write to a Go file with a compile error gets the
// language server's diagnostics folded into the tool result. Runs real gopls.
func TestWithEditDiagnostics_AppendsProblems(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed; skipping LSP integration test")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/edit\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	ci := codeintel.New(nil)
	t.Cleanup(func() { _ = ci.Close() })

	// A stub "write" tool that writes path+content under root (stands in for the
	// real fs write tool — we're testing the decorator, not fs).
	inner, _ := chat.NewTool(
		chat.ToolDefinition{Name: "write", Description: "stub", InputSchema: `{"type":"object"}`},
		chat.ToolMetadata{},
		func(_ context.Context, arguments string) (string, error) {
			var a struct{ Path, Content string }
			_ = json.Unmarshal([]byte(arguments), &a)
			if err := os.WriteFile(filepath.Join(root, a.Path), []byte(a.Content), 0o644); err != nil {
				return "", err
			}
			return "wrote " + a.Path, nil
		},
	)
	wrapped := withEditDiagnostics(inner, ci, root)
	args := `{"path":"oops.go","content":"package main\n\nfunc main() {\n\tundefinedXYZ()\n}\n"}`

	// Cold gopls may need more than one settle window; the file content is
	// stable across retries, so a late diagnostics push is read from cache.
	var out string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		o, err := wrapped.Call(context.Background(), args)
		if err != nil {
			t.Fatalf("wrapped write: %v", err)
		}
		if !strings.HasPrefix(o, "wrote oops.go") {
			t.Fatalf("inner result lost: %q", o)
		}
		if strings.Contains(o, "Language server flagged") {
			out = o
			break
		}
	}
	if out == "" {
		t.Fatal("no diagnostics section appended for a file with an undefined symbol")
	}
	if !strings.Contains(out, "error") || !strings.Contains(strings.ToLower(out), "undefined") {
		t.Errorf("diagnostics section = %q, want an 'undefined' error", out)
	}
}
