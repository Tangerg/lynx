package agentexec

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/chatclient"
)

// TestEngine_RegistersLSPTools verifies the code-intelligence tools are folded
// into the engine's tool set (so the model can call them): the combined `lsp`
// query tool + the separate `lsp_diagnostics`. This is a pure wiring check — no
// language server is started. The tool-layer behavior (unsupported file,
// post-edit diagnostics) is tested in internal/adapter/toolset.
func TestEngine_RegistersLSPTools(t *testing.T) {
	stub := newStubModel("nop", `{}`, "")
	client, _ := chatclient.New(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	have := map[string]bool{}
	for _, tool := range eng.catalog.Tools() {
		have[tool.Definition().Name] = true
	}
	for _, want := range []string{"lsp", "lsp_diagnostics"} {
		if !have[want] {
			t.Errorf("tool %q not registered in tool catalog", want)
		}
	}
}
