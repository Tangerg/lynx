package toolset

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

// TestMutationDiagnosticsAppendsProblems verifies the highest-value LSP
// integration: a successful mutation of a Go file with a compile error gets the
// language server's diagnostics folded into the tool result. Runs real gopls.
func TestMutationDiagnosticsAppendsProblems(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed; skipping LSP integration test")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/edit\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("go.mod: %v", err)
	}
	ci := newTestCodeIntel(t)

	// This stub isolates the decorator from the concrete filesystem adapter.
	type mutationArgs struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	inner, _ := toolcontract.NewFunc[mutationArgs, string](
		toolcontract.FuncConfig{Name: "test_mutation", Description: "stub"},
		func(_ context.Context, a mutationArgs) (string, error) {
			if err := os.WriteFile(filepath.Join(root, a.Path), []byte(a.Content), 0o644); err != nil {
				return "", err
			}
			return "wrote " + a.Path, nil
		},
	)
	wrapped := withMutationDiagnostics(inner, ci, root)
	args := `{"path":"oops.go","content":"package main\n\nfunc main() {\n\tundefinedXYZ()\n}\n"}`

	// Cold gopls may need more than one settle window; the file content is
	// stable across retries, so a late diagnostics push is read from cache.
	var out string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		o, err := wrapped.Call(context.Background(), args)
		if err != nil {
			t.Fatalf("wrapped mutation: %v", err)
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
