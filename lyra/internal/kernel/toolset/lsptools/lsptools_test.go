package lsptools

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/codeintel"
)

// lspTool returns the combined `lsp` tool from a fresh Build.
func lspTool(t *testing.T, ci *codeintel.Service) chat.Tool {
	t.Helper()
	for _, tool := range Build(ci, t.TempDir()) {
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

	out, err := lspTool(t, ci).Call(context.Background(), `{"operation":"hover","file":"notes.txt","line":1,"column":1}`)
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
		t.Error("definition without file must error")
	}
	if _, err := lsp.Call(context.Background(), `{"operation":"workspace_symbols"}`); err == nil {
		t.Error("workspace_symbols without query must error")
	}
	for _, op := range []string{"implementation", "incoming_calls", "outgoing_calls"} {
		out, err := lsp.Call(context.Background(), `{"operation":"`+op+`","file":"notes.txt","line":1,"column":1}`)
		if err != nil {
			t.Errorf("%s should not error on unsupported file: %v", op, err)
		}
		if !strings.Contains(out, "No language server") {
			t.Errorf("%s output = %q, want a no-server message", op, out)
		}
	}
}
