package kernel

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
)

// TestEngine_Tools_OfflineOnly verifies the engine exposes the
// always-on coding tool set when no Online credentials are
// configured. Provider-backed tools must NOT appear.
func TestEngine_Tools_OfflineOnly(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	tools := eng.Tools()
	// 8 filesystem/download coding tools + code_search + 3 shell tools (shell + its shell_output /
	// shell_kill companions) + 2 always-on LSP tools (the combined `lsp` query
	// tool + `lsp_diagnostics`) + the `task` delegation tool + the ask_user HITL
	// tool. (LSP tools advertise unconditionally; they return a no-server message
	// at call time when no language server applies.)
	if len(tools) != 16 {
		t.Fatalf("tool count = %d, want 16 (8 fs/download + code_search + 3 shell + 2 lsp + task + ask_user)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{
		"read", "write", "edit", "multiedit", "apply_patch", "download", "glob", "grep", "code_search", "shell", "task", "ask_user",
		"lsp", "lsp_diagnostics",
		"shell_output", "shell_kill",
	} {
		if !names[want] {
			t.Errorf("missing tool %q in %v", want, names)
		}
	}
	for _, never := range []string{"web_fetch", "web_search", "http_request"} {
		if names[never] {
			t.Errorf("unexpected online tool %q in offline build", never)
		}
	}
}

func TestEngine_New_WithoutResolverDoesNotInjectTask(t *testing.T) {
	t.Parallel()

	stub := newStubModel("shell", `{}`, "")
	client, _ := chat.NewClient(stub)
	customTool, err := chat.NewTool(
		chat.ToolDefinition{
			Name:        "noop",
			Description: "noop tool",
			InputSchema: `{"type":"object","properties":{}}`,
		},
		func(_ context.Context, _ string) (string, error) {
			return "noop", nil
		},
	)
	if err != nil {
		t.Fatalf("chat.NewTool: %v", err)
	}

	eng, err := New(context.Background(), Config{
		ChatClient: client,
		Tools:      []chat.Tool{customTool},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer eng.Close()

	tools := eng.Tools()
	names := toolNames(tools)
	if _, ok := names["task"]; ok {
		t.Fatalf("unexpected task tool without resolver: %v", names)
	}
	if _, ok := names["noop"]; !ok {
		t.Fatalf("custom tool should be preserved, names=%v", names)
	}
}

// TestEngine_Tools_OnlineEnabled verifies provider-backed tools
// arrive when their credentials are supplied.
func TestEngine_Tools_OnlineEnabled(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		Online: toolset.OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"api.example.com"},
		},
	})
	defer eng.Close()

	tools := eng.Tools()
	if len(tools) != 19 {
		t.Fatalf("tool count = %d, want 19 (8 fs/download + code_search + 3 shell + 2 lsp + 3 online + task + ask_user)", len(tools))
	}
	names := toolNames(tools)
	for _, want := range []string{"web_fetch", "web_search", "http_request"} {
		if !names[want] {
			t.Errorf("expected online tool %q in %v", want, names)
		}
	}
}

// TestEngine_Tools_PartialOnline verifies each online tool is
// independent -- supplying only one credential registers only one
// extra tool.
func TestEngine_Tools_PartialOnline(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{Online: toolset.OnlineConfig{JinaAPIKey: "k"}})
	defer eng.Close()
	if len(eng.Tools()) != 17 {
		t.Fatalf("tool count = %d, want 17 (8 fs/download + code_search + 3 shell + 2 lsp + jina + task + ask_user)", len(eng.Tools()))
	}
}

func toolNames(tools []chat.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Definition().Name] = true
	}
	return out
}
