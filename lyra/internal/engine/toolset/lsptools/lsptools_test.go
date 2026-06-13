package lsptools

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/codeintel"
)

// TestLSPToolUnsupportedFile checks the tool-layer contract: a query on a file
// type with no configured server returns a plain message (the model adapts),
// not an error that would halt the loop.
func TestLSPToolUnsupportedFile(t *testing.T) {
	ci := codeintel.New(nil)
	t.Cleanup(func() { _ = ci.Close() })
	tools := Build(ci, t.TempDir())

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
