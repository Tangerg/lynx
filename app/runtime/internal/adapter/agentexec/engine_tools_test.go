package agentexec

import (
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/tools"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
)

func TestToolCatalogReturnsSnapshot(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	first := codingTools(t, eng.catalog)
	if len(first) == 0 {
		t.Fatal("catalog has no tools")
	}
	first[0] = nil
	if second := codingTools(t, eng.catalog); second[0] == nil {
		t.Fatal("catalog exposed its mutable backing slice")
	}
}

// TestToolCatalogOfflineOnly verifies the assembled catalog exposes the
// always-on coding tool set when no Online credentials are
// configured. Provider-backed tools must NOT appear.
func TestToolCatalogOfflineOnly(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	tools := codingTools(t, eng.catalog)
	// 6 filesystem coding tools (download is gated on a host allowlist, so it is
	// absent in this offline build) + 3 shell tools (shell + its shell_output /
	// shell_kill companions) + 2 always-on LSP tools (the combined `lsp` query
	// tool + `lsp_diagnostics`) + the `task` delegation tool + the ask_user HITL
	// tool. (LSP tools advertise unconditionally; they return a no-server message
	// at call time when no language server applies.)
	if len(tools) != 13 {
		t.Fatalf("tool count = %d, want 13 (6 fs + 3 shell + 2 lsp + task + ask_user)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{
		"read", "write", "edit", "apply_patch", "glob", "grep", "shell", "task", "ask_user",
		"lsp", "lsp_diagnostics",
		"shell_output", "shell_kill",
	} {
		if !names[want] {
			t.Errorf("missing tool %q in %v", want, names)
		}
	}
	// download joins the online tools as allowlist-gated: absent without one.
	for _, never := range []string{"web_fetch", "web_search", "http_request", "download"} {
		if names[never] {
			t.Errorf("unexpected online tool %q in offline build", never)
		}
	}
}

// TestToolCatalogOnlineEnabled verifies provider-backed tools
// arrive when their credentials are supplied.
func TestToolCatalogOnlineEnabled(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		Online: toolset.OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"api.example.com"},
		},
	})
	defer eng.Close()

	tools := codingTools(t, eng.catalog)
	if len(tools) != 17 {
		t.Fatalf("tool count = %d, want 17 (6 fs + download + 3 shell + 2 lsp + 3 online + task + ask_user)", len(tools))
	}
	names := toolNames(tools)
	// HTTPAllowedHosts is set, so download is registered alongside the online tools.
	for _, want := range []string{"web_fetch", "web_search", "http_request", "download"} {
		if !names[want] {
			t.Errorf("expected online tool %q in %v", want, names)
		}
	}
}

// TestToolCatalogPartialOnline verifies each online tool is
// independent -- supplying only one credential registers only one
// extra tool.
func TestToolCatalogPartialOnline(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{Online: toolset.OnlineConfig{JinaAPIKey: "k"}})
	defer eng.Close()
	if got := len(codingTools(t, eng.catalog)); got != 14 {
		t.Fatalf("tool count = %d, want 14 (6 fs + 3 shell + 2 lsp + jina + task + ask_user; no download without an http allowlist)", got)
	}
}

func toolNames(tools []tools.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Definition().Name] = true
	}
	return out
}
