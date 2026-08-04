package agentexec

import (
	"encoding/json"
	"strings"
	"testing"

	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/chatclient"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset/catalog"
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
			if candidate.Definition().Name == catalog.DelegateTask {
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
	for _, forbidden := range []string{"ui", "frontend", "runtime", "chip", "button"} {
		if strings.Contains(strings.ToLower(def.Description), forbidden) {
			t.Fatalf("delegate_task description leaks %q: %q", forbidden, def.Description)
		}
	}
	for name, arguments := range map[string]string{
		"obsolete field":      `{"summary":"focused task","instructions":"inspect the package","prompt":"legacy"}`,
		"empty summary":       `{"summary":"","instructions":"inspect the package"}`,
		"padded summary":      `{"summary":" focused task ","instructions":"inspect the package"}`,
		"overlong summary":    `{"summary":"` + strings.Repeat("a", 81) + `","instructions":"inspect the package"}`,
		"missing field":       `{"summary":"focused task"}`,
		"trailing JSON value": `{"summary":"focused task","instructions":"inspect the package"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := definition.Call(t.Context(), arguments)
			if err == nil {
				t.Fatal("delegate_task accepted arguments outside its model-visible contract")
			}
			if !strings.Contains(err.Error(), "decode function arguments") {
				t.Fatalf("delegate_task reached Agent execution before input rejection: %v", err)
			}
		})
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
	// Four filesystem tools (read, glob, grep, and apply_patch)
	// + 3 shell tools (shell + its read_shell_output /
	// stop_shell companions) + one deferred `lsp` operation tool + the
	// `delegate_task` tool + the ask_user HITL
	// tool + search_tools. LSP remains executable but is deferred from the model's
	// initial manifest.
	if len(tools) != 11 {
		t.Fatalf("tool count = %d, want 11 (4 fs + 3 shell + lsp + delegate_task + ask_user + search_tools)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{
		catalog.Read, catalog.Glob, catalog.Grep, catalog.ApplyPatch, catalog.Shell, catalog.DelegateTask,
		catalog.AskUser, catalog.SearchTools, catalog.LSP, catalog.ReadShellOutput, catalog.StopShell,
	} {
		if !names[want] {
			t.Errorf("missing tool %q in %v", want, names)
		}
	}
	if names["edit"] || names["write"] {
		t.Fatal("Runtime must expose only apply_patch for filesystem mutation")
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
	if len(tools) != 14 {
		t.Fatalf("tool count = %d, want 14 (4 fs + 3 shell + lsp + 3 online + delegate_task + ask_user + search_tools)", len(tools))
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
	if got := len(codingTools(t, eng.catalog)); got != 12 {
		t.Fatalf("tool count = %d, want 12 (4 fs + 3 shell + lsp + jina + delegate_task + ask_user + search_tools)", got)
	}
}

func toolNames(tools []toolcontract.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Definition().Name] = true
	}
	return out
}
