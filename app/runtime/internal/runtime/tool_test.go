package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type toolCatalog struct {
	tools []tool.Tool
}

func (c toolCatalog) List(context.Context) ([]tool.Tool, error) {
	return c.tools, nil
}

type toolInvoker struct {
	name      string
	arguments string
}

func (i *toolInvoker) Invoke(_ context.Context, name string, arguments string) (string, error) {
	i.name = name
	i.arguments = arguments
	return "ok", nil
}

func TestRuntimeListRegisteredToolsUsesCatalogPort(t *testing.T) {
	rt := &Runtime{toolCatalog: toolCatalog{tools: []tool.Tool{{Name: "read"}}}}

	got, err := rt.ListRegisteredTools(context.Background())
	if err != nil {
		t.Fatalf("ListRegisteredTools: %v", err)
	}
	if len(got) != 1 || got[0].Name != "read" {
		t.Fatalf("tools = %+v, want read", got)
	}
}

func TestRuntimeInvokeRegisteredToolUsesInvocationPort(t *testing.T) {
	invoker := &toolInvoker{}
	rt := &Runtime{toolInvocations: invoker}

	got, err := rt.InvokeRegisteredTool(context.Background(), "shell", `{"command":"true"}`)
	if err != nil {
		t.Fatalf("InvokeRegisteredTool: %v", err)
	}
	if got != "ok" || invoker.name != "shell" || invoker.arguments != `{"command":"true"}` {
		t.Fatalf("result=%q name=%q arguments=%q", got, invoker.name, invoker.arguments)
	}
}
