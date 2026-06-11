package engine

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
	"github.com/Tangerg/lynx/lyra/internal/lsp"
)

// TestEngine_RegistersLSPTools verifies the six code-intelligence tools are
// folded into the engine's tool set (so the model can call them). This is a
// pure wiring check — no language server is started.
func TestEngine_RegistersLSPTools(t *testing.T) {
	stub := newStubModel("nop", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	have := map[string]bool{}
	for _, tool := range eng.Tools() {
		have[tool.Definition().Name] = true
	}
	for _, want := range []string{
		"lsp_definition", "lsp_references", "lsp_hover",
		"lsp_document_symbols", "lsp_diagnostics", "lsp_workspace_symbols",
	} {
		if !have[want] {
			t.Errorf("tool %q not registered in engine.Tools()", want)
		}
	}
}

// TestLSPToolUnsupportedFile checks the tool-layer contract: a query on a file
// type with no configured server returns a plain message (the model adapts),
// not an error that would halt the loop.
func TestLSPToolUnsupportedFile(t *testing.T) {
	mgr := lsp.NewManager(lsp.DefaultServers())
	t.Cleanup(func() { _ = mgr.Close() })
	tools := buildLSPTools(mgr, t.TempDir())

	var hover chat.Tool
	for _, tool := range tools {
		if tool.Definition().Name == "lsp_hover" {
			hover = tool
		}
	}
	if hover == nil {
		t.Fatal("lsp_hover tool not built")
	}
	out, err := hover.Call(context.Background(), `{"file":"notes.txt","line":1,"column":1}`)
	if err != nil {
		t.Fatalf("unsupported file should not error: %v", err)
	}
	if !strings.Contains(out, "No language server") {
		t.Errorf("output = %q, want a no-server message", out)
	}
}

// TestNewProblems_FiltersBaseline is the deterministic guard against the LSP
// caching / staleness false positives the baseline diff exists to prevent: a
// pre-existing problem (even one whose line shifted, or one the server
// re-reported verbatim from cache) must not be counted as introduced — only a
// genuinely new diagnostic surfaces.
func TestNewProblems_FiltersBaseline(t *testing.T) {
	before := []lsp.Diagnostic{
		{Message: "undefined: foo", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 10}}},
	}
	after := []lsp.Diagnostic{
		// same problem, shifted down 5 lines + re-reported → must be filtered
		{Message: "undefined: foo", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 15}}},
		// genuinely new problem the edit introduced
		{Message: "undefined: bar", Severity: 1, Range: lsp.Range{Start: lsp.Position{Line: 20}}},
	}
	got := newProblems(before, after)
	if len(got) != 1 || got[0].Message != "undefined: bar" {
		t.Fatalf("newProblems = %v, want only [undefined: bar]", got)
	}

	// A cached, identical result (after == before) yields nothing.
	if n := newProblems(before, before); len(n) != 0 {
		t.Errorf("newProblems(before, before) = %v, want none (stale cache must not false-positive)", n)
	}
}

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
	mgr := lsp.NewManager(lsp.DefaultServers())
	t.Cleanup(func() { _ = mgr.Close() })

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
	wrapped := withEditDiagnostics(inner, mgr, root)
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
