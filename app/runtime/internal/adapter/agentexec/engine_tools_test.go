package agentexec

import (
	"encoding/json"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/chatclient"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
)

func TestToolCatalogReturnsSnapshot(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub, chatclient.Config{})
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

func TestDelegationToolUsesOnePreciseContract(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	var definition = func() (found toolcontract.Tool) {
		for _, candidate := range codingTools(t, eng.catalog) {
			if candidate.Definition().Name == "delegate_task" {
				return candidate
			}
		}
		return nil
	}()
	if definition == nil {
		t.Fatal("delegate_task not registered")
	}
	def := definition.Definition()
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	for _, name := range []string{"summary", "instructions"} {
		if _, ok := schema.Properties[name]; !ok || !containsString(schema.Required, name) {
			t.Errorf("delegate_task schema = %s, want required %q", def.InputSchema, name)
		}
	}
	for _, obsolete := range []string{"description", "prompt"} {
		if _, ok := schema.Properties[obsolete]; ok {
			t.Errorf("delegate_task schema retains obsolete %q: %s", obsolete, def.InputSchema)
		}
	}
	if strings.Contains(strings.ToLower(def.Description), "ui") {
		t.Fatalf("delegate_task description leaks presentation concerns: %q", def.Description)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// TestToolCatalogOfflineOnly verifies the assembled catalog exposes the
// always-on coding tool set when no Online credentials are
// configured. Provider-backed tools must NOT appear.
func TestToolCatalogOfflineOnly(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	tools := codingTools(t, eng.catalog)
	// 5 filesystem coding tools in the conservative edit/write vocabulary
	// + 3 shell tools (shell + its read_shell_output /
	// stop_shell companions) + one deferred `lsp` operation tool + the
	// `delegate_task` tool + the ask_user HITL
	// tool + search_tools. LSP remains executable but is deferred from the model's
	// initial manifest.
	if len(tools) != 12 {
		t.Fatalf("tool count = %d, want 12 (5 fs + 3 shell + lsp + delegate_task + ask_user + search_tools)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{
		"read", "write", "edit", "glob", "grep", "shell", "delegate_task", "ask_user", "search_tools", "lsp",
		"read_shell_output", "stop_shell",
	} {
		if !names[want] {
			t.Errorf("missing tool %q in %v", want, names)
		}
	}
	if names["apply_patch"] {
		t.Fatal("one Run must not register apply_patch together with edit/write")
	}
	for _, never := range []string{"web_fetch", "web_search", "http_request"} {
		if names[never] {
			t.Errorf("unexpected online tool %q in offline build", never)
		}
	}
}

// TestToolCatalogOnlineEnabled verifies provider-backed tools
// arrive when their credentials are supplied.
func TestToolCatalogOnlineEnabled(t *testing.T) {
	stub := newStubModel("shell", `{}`, "")
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{
		Online: toolset.OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"api.example.com"},
		},
	})
	defer eng.Close()

	tools := codingTools(t, eng.catalog)
	if len(tools) != 15 {
		t.Fatalf("tool count = %d, want 15 (5 fs + 3 shell + lsp + 3 online + delegate_task + ask_user + search_tools)", len(tools))
	}
	names := toolNames(tools)
	for _, want := range []string{"web_fetch", "web_search", "http_request"} {
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
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{Online: toolset.OnlineConfig{JinaAPIKey: "k"}})
	defer eng.Close()
	if got := len(codingTools(t, eng.catalog)); got != 13 {
		t.Fatalf("tool count = %d, want 13 (5 fs + 3 shell + lsp + jina + delegate_task + ask_user + search_tools)", got)
	}
}

func toolNames(tools []toolcontract.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Definition().Name] = true
	}
	return out
}
