package builtin

import (
	"context"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/codeintel"
)

// lspTool returns the combined `lsp` tool from a fresh Build.
func lspTool(t *testing.T, ci *codeintel.Analyzer) toolcontract.Tool {
	t.Helper()
	tools, err := BuildLSP(ci, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Definition().Name == "lsp" {
			return tool
		}
	}
	t.Fatal("lsp tool not built")
	return nil
}

// TestLSPToolUnsupportedFile checks the tool-layer contract: a query on a file
// type with no configured server returns a plain message (the model adapts),
// not an error that would halt the loop.
func TestLSPToolUnsupportedFile(t *testing.T) {
	ci := codeintel.New(nil)
	t.Cleanup(func() { _ = ci.Close() })

	out, err := lspTool(t, ci).Call(context.Background(), `{"operation":"hover","path":"notes.txt","line":1,"character":1}`)
	if err != nil {
		t.Fatalf("unsupported file should not error: %v", err)
	}
	if !strings.Contains(out, "No language server") {
		t.Errorf("output = %q, want a no-server message", out)
	}
}

// TestLSPToolValidation covers the combined tool's dispatch guards: an unknown
// operation and a missing required operand are model-facing errors, and the new
// operations (implementation, incoming/outgoing calls) are accepted + routed
// (returning the no-server message under the default servers, not an error).
func TestLSPToolValidation(t *testing.T) {
	ci := codeintel.New(nil)
	t.Cleanup(func() { _ = ci.Close() })
	lsp := lspTool(t, ci)

	if _, err := lsp.Call(context.Background(), `{"operation":"bogus"}`); err == nil {
		t.Error("unknown operation must error")
	}
	if _, err := lsp.Call(context.Background(), `{"operation":"definition"}`); err == nil {
		t.Error("definition without path must error")
	}
	if _, err := lsp.Call(context.Background(), `{"operation":"definition","path":"notes.txt"}`); err == nil {
		t.Error("position operation without line and character must error")
	}
	if _, err := lsp.Call(context.Background(), `{"operation":"workspace_symbols"}`); err == nil {
		t.Error("workspace_symbols without query must error")
	}
	for _, op := range []string{"implementation", "incoming_calls", "outgoing_calls"} {
		out, err := lsp.Call(context.Background(), `{"operation":"`+op+`","path":"notes.txt","line":1,"character":1}`)
		if err != nil {
			t.Errorf("%s should not error on unsupported file: %v", op, err)
		}
		if !strings.Contains(out, "No language server") {
			t.Errorf("%s output = %q, want a no-server message", op, out)
		}
	}
	if out, err := lsp.Call(context.Background(), `{"operation":"diagnostics","path":"notes.txt"}`); err != nil || !strings.Contains(out, "No language server") {
		t.Errorf("diagnostics = (%q, %v), want a no-server message", out, err)
	}
	if _, err := lsp.Call(context.Background(), `{"operation":"diagnostics","file_path":"notes.txt"}`); err == nil {
		t.Error("obsolete file_path field must be rejected")
	}
	for _, arguments := range []string{
		`{"operation":"definition","path":"notes.txt","line":1,"character":1,"query":"unused"}`,
		`{"operation":"diagnostics","path":"notes.txt","line":1}`,
		`{"operation":"document_symbols","path":"notes.txt","query":"unused"}`,
		`{"operation":"workspace_symbols","query":"Thing","path":"notes.txt"}`,
	} {
		if _, err := lsp.Call(context.Background(), arguments); err == nil {
			t.Errorf("lsp accepted fields ignored by the selected operation: %s", arguments)
		}
	}
}
