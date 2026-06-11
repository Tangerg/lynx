package engine

import (
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/engine/toolset"
)

// TestEngine_RegistersLSPTools verifies the six code-intelligence tools are
// folded into the engine's tool set (so the model can call them). This is a
// pure wiring check — no language server is started. The tool-layer behavior
// (unsupported file, post-edit diagnostics) is tested in internal/engine/toolset.
func TestEngine_RegistersLSPTools(t *testing.T) {
	stub := newStubModel("nop", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
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
