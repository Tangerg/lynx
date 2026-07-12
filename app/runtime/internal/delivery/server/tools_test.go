package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// toolRegistryFake is the tool.Registry the tools coordinator drives.
type toolRegistryFake struct {
	tools          []tool.Tool
	invokedName    string
	invokedPayload string
}

func (r *toolRegistryFake) List(context.Context) ([]tool.Tool, error) { return r.tools, nil }

func (r *toolRegistryFake) Invoke(_ context.Context, name string, arguments string) (string, error) {
	r.invokedName = name
	r.invokedPayload = arguments
	return "ok", nil
}

func TestListToolsMapsRegisteredToolsToWire(t *testing.T) {
	s := serverWithTools(&toolRegistryFake{tools: []tool.Tool{
		{
			Name:        "shell",
			Description: "run a command",
			Schema:      `{"type":"object","properties":{"cmd":{"type":"string"}}}`,
			SafetyClass: tool.SafetyClassExec,
		},
		{
			Name:        "bad-schema",
			Description: "still listed",
			Schema:      `{`,
			SafetyClass: tool.SafetyClassSafe,
		},
	}})

	page, err := s.ListTools(context.Background(), protocol.PageQuery{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("tools = %+v, want 2", page.Data)
	}
	if page.Data[0].Name != "shell" || page.Data[0].SafetyClass != protocol.SafetyClassExec {
		t.Fatalf("shell wire = %+v", page.Data[0])
	}
	if page.Data[0].Parameters["type"] != "object" {
		t.Fatalf("schema = %+v, want decoded object schema", page.Data[0].Parameters)
	}
	if len(page.Data[1].Parameters) != 0 {
		t.Fatalf("bad schema parameters = %+v, want empty object", page.Data[1].Parameters)
	}
}

func TestInvokeToolPassesJSONArgumentsToRuntime(t *testing.T) {
	rt := &toolRegistryFake{}
	s := serverWithTools(rt)

	got, err := s.InvokeTool(context.Background(), protocol.InvokeToolRequest{
		Name:      "shell",
		Arguments: map[string]any{"cmd": "pwd"},
	})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %v, want ok", got)
	}
	if rt.invokedName != "shell" {
		t.Fatalf("invokedName = %q, want shell", rt.invokedName)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(rt.invokedPayload), &payload); err != nil {
		t.Fatalf("payload %q is not JSON: %v", rt.invokedPayload, err)
	}
	if payload["cmd"] != "pwd" {
		t.Fatalf("payload = %+v, want cmd=pwd", payload)
	}
}
