package engine

import (
	"context"
	"strings"
	"testing"

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
