package tool_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/domain/tool"
	"github.com/Tangerg/lynx/lyra/internal/kernel"
	"github.com/Tangerg/lynx/lyra/internal/kernel/toolset"
)

// TestService_List enumerates the coding tool set and verifies the
// SafetyClass mapping matches the documented defaults.
func TestService_List(t *testing.T) {
	svc := buildService(t)

	tools, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantClasses := map[string]tool.SafetyClass{
		"read":  tool.SafetyClassSafe,
		"glob":  tool.SafetyClassSafe,
		"grep":  tool.SafetyClassSafe,
		"write": tool.SafetyClassWrite,
		"edit":  tool.SafetyClassWrite,
		"bash":  tool.SafetyClassExec,
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

// TestService_Invoke_Bash runs `bash` directly (no agent loop) and
// asserts the output reflects the command we asked for. Useful as a
// smoke test of the headless-invocation path.
func TestService_Invoke_Bash(t *testing.T) {
	svc := buildService(t)
	out, err := svc.Invoke(context.Background(), "bash", `{"command":"echo lyra"}`)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(out, "lyra") {
		t.Errorf("Invoke output missing 'lyra': %q", out)
	}
}

// TestService_Invoke_UnknownTool rejects with a clear error so
// callers (and transport adapters) can present it intact.
func TestService_Invoke_UnknownTool(t *testing.T) {
	svc := buildService(t)
	if _, err := svc.Invoke(context.Background(), "no-such-tool", "{}"); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildService(t *testing.T) tool.Service {
	t.Helper()
	client, err := chatmodel.NewClient(newStubModel())
	if err != nil {
		t.Fatal(err)
	}
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := kernel.New(context.Background(), kernel.Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Tools:        built.Tools,
		MCP:          built.MCP,
		Closers:      built.Closers,
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := tool.New(eng)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

type stubModel struct{ defaults *chatmodel.Options }

func newStubModel() *stubModel {
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubModel{defaults: opts}
}

func (m *stubModel) DefaultOptions() chatmodel.Options { return *m.defaults }
func (m *stubModel) Metadata() chatmodel.ModelMetadata {
	return chatmodel.ModelMetadata{Provider: "stub"}
}

func (m *stubModel) Call(_ context.Context, _ *chatmodel.Request) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage("nop"),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func (m *stubModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}
