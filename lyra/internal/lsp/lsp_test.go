package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeModule lays down a tiny buildable Go module and returns its root. The
// LSP operations are exercised against real gopls, so the fixture must be a
// genuine module (gopls keys everything off go.mod).
func writeModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("go.mod", "module example.com/lsptest\n\ngo 1.21\n")
	// Line/char references below are 0-based, matching the wire.
	write("main.go", "package main\n"+ // 0
		"\n"+ // 1
		"func Greet(name string) string {\n"+ // 2: decl of Greet at char 5
		"\treturn \"hi \" + name\n"+ // 3
		"}\n"+ // 4
		"\n"+ // 5
		"func main() {\n"+ // 6
		"\t_ = Greet(\"world\")\n"+ // 7: use of Greet at char 5
		"}\n") // 8
	write("bad.go", "package main\n\nfunc bad() {\n\tundefinedSymbol()\n}\n")
	return root
}

// gopls is at minimum 3s of cold start; give the whole suite generous slack.
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed; skipping LSP integration test")
	}
	mgr := NewManager(DefaultServers())
	t.Cleanup(func() { _ = mgr.Close() })
	return mgr
}

func TestManager_DefinitionAndHover(t *testing.T) {
	mgr := newTestManager(t)
	root := writeModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Definition of Greet at its use site (main.go:7:5) → its declaration (line 2).
	locs, err := mgr.Definition(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("Definition returned no locations")
	}
	if got := locs[0].Range.Start.Line; got != 2 {
		t.Errorf("Definition start line = %d, want 2", got)
	}
	if !strings.HasSuffix(uriToPath(locs[0].URI), "main.go") {
		t.Errorf("Definition uri = %q, want .../main.go", locs[0].URI)
	}

	// Hover at the same spot mentions the function.
	hov, err := mgr.Hover(ctx, root, "main.go", Position{Line: 7, Character: 5})
	if err != nil {
		t.Fatalf("Hover: %v", err)
	}
	if !strings.Contains(hov, "Greet") {
		t.Errorf("Hover = %q, want it to mention Greet", hov)
	}
}

func TestManager_ReferencesAndSymbols(t *testing.T) {
	mgr := newTestManager(t)
	root := writeModule(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// References to Greet (declaration at main.go:2:5) → declaration + the call.
	refs, err := mgr.References(ctx, root, "main.go", Position{Line: 2, Character: 5})
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if len(refs) < 2 {
		t.Errorf("References = %d, want >= 2 (declaration + use)", len(refs))
	}

	// Document symbols include both functions.
	syms, err := mgr.DocumentSymbols(ctx, root, "main.go")
	if err != nil {
		t.Fatalf("DocumentSymbols: %v", err)
	}
	if !hasSymbol(syms, "Greet") || !hasSymbol(syms, "main") {
		t.Errorf("DocumentSymbols = %v, want Greet and main", names(syms))
	}

	// Workspace symbol search finds Greet across the module.
	ws, err := mgr.WorkspaceSymbols(ctx, root, "Greet")
	if err != nil {
		t.Fatalf("WorkspaceSymbols: %v", err)
	}
	if !hasSymbol(ws, "Greet") {
		t.Errorf("WorkspaceSymbols(Greet) = %v, want Greet", names(ws))
	}
}

func TestManager_Diagnostics(t *testing.T) {
	mgr := newTestManager(t)
	root := writeModule(t)

	// Cold gopls can take longer than one settle window to first analyze; poll.
	var diags []Diagnostic
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		d, err := mgr.Diagnostics(ctx, root, "bad.go")
		cancel()
		if err != nil {
			t.Fatalf("Diagnostics: %v", err)
		}
		if len(d) > 0 {
			diags = d
			break
		}
	}
	if len(diags) == 0 {
		t.Fatal("Diagnostics returned none for a file with an undefined symbol")
	}
	if diags[0].SeverityName() != "error" {
		t.Errorf("diagnostic severity = %q, want error", diags[0].SeverityName())
	}
}

func TestManager_UnsupportedFile(t *testing.T) {
	mgr := NewManager(DefaultServers())
	t.Cleanup(func() { _ = mgr.Close() })
	if mgr.Supported("notes.txt") {
		t.Error("Supported(.txt) = true, want false")
	}
	if mgr.Supported("main.go") != true {
		t.Error("Supported(.go) = false, want true")
	}
	_, err := mgr.Definition(context.Background(), t.TempDir(), "notes.txt", Position{})
	if err == nil {
		t.Fatal("Definition on unsupported file should error")
	}
}

func hasSymbol(syms []Symbol, name string) bool {
	for _, s := range syms {
		if s.Name == name {
			return true
		}
	}
	return false
}

func names(syms []Symbol) []string {
	var out []string
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}
