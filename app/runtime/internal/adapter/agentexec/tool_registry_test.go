package agentexec_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatclient"
	chatmodel "github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// TestRegistry_List enumerates the coding tool set and verifies the
// SafetyClass mapping matches the documented defaults.
func TestRegistry_List(t *testing.T) {
	reg := buildRegistry(t)

	tools, err := reg.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantClasses := map[string]tool.SafetyClass{
		"read":        tool.SafetyClassSafe,
		"glob":        tool.SafetyClassSafe,
		"grep":        tool.SafetyClassSafe,
		"write":       tool.SafetyClassWrite,
		"edit":        tool.SafetyClassWrite,
		"apply_patch": tool.SafetyClassWrite,
		"shell":       tool.SafetyClassExec,
	}

	got := map[string]tool.SafetyClass{}
	for _, tl := range tools {
		got[tl.Name] = tl.SafetyClass
		if tl.Schema == "" {
			t.Errorf("tool %q has empty schema", tl.Name)
		}
		if tl.Description == "" {
			t.Errorf("tool %q has empty description", tl.Name)
		}
	}

	for name, wantClass := range wantClasses {
		if got[name] != wantClass {
			t.Errorf("tool %q safety = %d, want %d", name, got[name], wantClass)
		}
	}
}

// TestRegistry_Invoke_Shell runs `shell` directly (no agent loop) and
// asserts the output reflects the command we asked for. Useful as a
// smoke test of the headless-invocation path.
func TestRegistry_Invoke_Shell(t *testing.T) {
	reg := buildRegistry(t)
	out, err := reg.Invoke(context.Background(), "shell", `{"command":"echo lyra"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(out, "lyra") {
		t.Errorf("Invoke output missing 'lyra': %q", out)
	}
}

// TestRegistry_Invoke_UnknownTool rejects with a clear error so
// callers (and transport adapters) can present it intact.
func TestRegistry_Invoke_UnknownTool(t *testing.T) {
	reg := buildRegistry(t)
	if _, err := reg.Invoke(context.Background(), "no-such-tool", "{}"); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildRegistry(t *testing.T) tool.Registry {
	t.Helper()
	client, err := chatclient.New(newStubModel())
	if err != nil {
		t.Fatal(err)
	}
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := agentexec.New(context.Background(), agentexec.Config{
		ChatClient:            client,
		ToolResolver:          built.Resolver,
		Tools:                 built.Tools,
		MCPStatusReader:       built.MCPStatusReader,
		MCPToolCatalog:        built.MCPToolCatalog,
		MCPConnectionCommands: built.MCPConnectionCommands,
		MCPRegistryCommands:   built.MCPRegistryCommands,
		Closers:               built.Closers,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := agentexec.NewToolRegistry(eng)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

type stubModel struct{}

func newStubModel() *stubModel { return &stubModel{} }

func (m *stubModel) Call(_ context.Context, _ *chatmodel.Request) (*chatmodel.Response, error) {
	message := chatmodel.NewAssistantMessage(chatmodel.NewTextPart("nop"))
	return chatmodel.NewResponse(chatmodel.Choice{Index: 0, Message: &message, FinishReason: chatmodel.FinishReasonStop})
}

func (m *stubModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}
