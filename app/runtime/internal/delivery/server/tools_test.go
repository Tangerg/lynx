package server

import (
	"context"
	"encoding/json"
	"testing"

	toolapp "github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// toolRegistryFake is the diagnostic tool registry the tools coordinator drives.
type toolRegistryFake struct {
	tools          []tool.Tool
	invokedCwd     string
	invokedName    string
	invokedPayload string
}

func (r *toolRegistryFake) List(context.Context) ([]tool.Tool, error) { return r.tools, nil }

func (r *toolRegistryFake) Invoke(_ context.Context, in toolapp.Invocation) (tool.Result, error) {
	r.invokedCwd = in.Cwd
	r.invokedName = in.Name
	r.invokedPayload = in.Arguments
	return tool.StringResult("ok"), nil
}

func TestListToolsMapsRegisteredToolsToWire(t *testing.T) {
	shellSchema, err := tool.ParseSchema([]byte(`{"type":"object","properties":{"cmd":{"type":"string"}}}`))
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	s := serverWithTools(&toolRegistryFake{tools: []tool.Tool{
		{
			Name:        "shell",
			Description: "run a command",
			Schema:      shellSchema,
			SafetyClass: tool.SafetyClassExec,
		},
	}})

	page, err := s.ListTools(context.Background(), protocol.PageQuery{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("tools = %+v, want 1", page.Data)
	}
	if page.Data[0].Name != "shell" || page.Data[0].SafetyClass != protocol.SafetyClassExec {
		t.Fatalf("shell wire = %+v", page.Data[0])
	}
	if page.Data[0].Parameters["type"] != "object" {
		t.Fatalf("schema = %+v, want decoded object schema", page.Data[0].Parameters)
	}
}

func TestInvokeToolPassesJSONArgumentsToRuntime(t *testing.T) {
	rt := &toolRegistryFake{}
	s := serverWithTools(rt)

	got, err := s.InvokeTool(context.Background(), protocol.InvokeToolRequest{
		Name:      "read",
		Arguments: map[string]any{"file_path": "main.go"},
		Cwd:       "/workspace",
	})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %v, want ok", got)
	}
	if rt.invokedName != "read" || rt.invokedCwd != "/workspace" {
		t.Fatalf("invocation = %q in %q, want read in /workspace", rt.invokedName, rt.invokedCwd)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(rt.invokedPayload), &payload); err != nil {
		t.Fatalf("payload %q is not JSON: %v", rt.invokedPayload, err)
	}
	if payload["file_path"] != "main.go" {
		t.Fatalf("payload = %+v, want file_path=main.go", payload)
	}
}
