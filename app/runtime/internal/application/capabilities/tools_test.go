package capabilities

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type toolRegistryFixture struct {
	tools []tool.Tool
}

func (c toolRegistryFixture) List(context.Context) ([]tool.Tool, error) { return c.tools, nil }
func (toolRegistryFixture) Invoke(context.Context, string, string) (string, error) {
	return "", nil
}

type toolRegistryRecorder struct {
	name      string
	arguments string
}

func (i *toolRegistryRecorder) Invoke(_ context.Context, name string, arguments string) (string, error) {
	i.name = name
	i.arguments = arguments
	return "ok", nil
}

func (*toolRegistryRecorder) List(context.Context) ([]tool.Tool, error) { return nil, nil }

func TestListRegisteredToolsUsesRegistry(t *testing.T) {
	c := New(Config{Tools: toolRegistryFixture{tools: []tool.Tool{{Name: "read"}}}})

	got, err := c.ListRegisteredTools(context.Background())
	if err != nil {
		t.Fatalf("ListRegisteredTools: %v", err)
	}
	if len(got) != 1 || got[0].Name != "read" {
		t.Fatalf("tools = %+v, want read", got)
	}
}

func TestInvokeRegisteredToolUsesRegistry(t *testing.T) {
	invoker := &toolRegistryRecorder{}
	c := New(Config{Tools: invoker})

	got, err := c.InvokeRegisteredTool(context.Background(), "shell", `{"command":"true"}`)
	if err != nil {
		t.Fatalf("InvokeRegisteredTool: %v", err)
	}
	if got != "ok" || invoker.name != "shell" || invoker.arguments != `{"command":"true"}` {
		t.Fatalf("result=%q name=%q arguments=%q", got, invoker.name, invoker.arguments)
	}
}
