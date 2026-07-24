package tools

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type toolRegistryFixture struct {
	tools []tool.Tool
}

func (c toolRegistryFixture) List(context.Context) ([]tool.Tool, error) { return c.tools, nil }
func (toolRegistryFixture) Invoke(context.Context, string, string, string) (tool.Result, error) {
	return tool.Result{}, nil
}

type toolRegistryRecorder struct {
	root      string
	name      string
	arguments string
}

func (i *toolRegistryRecorder) Invoke(_ context.Context, root, name string, arguments string) (tool.Result, error) {
	i.root = root
	i.name = name
	i.arguments = arguments
	return tool.StringResult("ok"), nil
}

func (*toolRegistryRecorder) List(context.Context) ([]tool.Tool, error) { return nil, nil }

type rootRecorder struct {
	root string
}

func (r *rootRecorder) ResolveRoot(cwd string) (string, error) {
	r.root = cwd
	return "/workspace", nil
}

func TestListUsesRegistry(t *testing.T) {
	c := New(toolRegistryFixture{tools: []tool.Tool{{Name: "read"}}}, &rootRecorder{})

	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "read" {
		t.Fatalf("tools = %+v, want read", got)
	}
}

func TestInvokeUsesRegistry(t *testing.T) {
	invoker := &toolRegistryRecorder{}
	roots := &rootRecorder{}
	c := New(invoker, roots)

	got, err := c.Invoke(context.Background(), Invocation{Name: "read", Arguments: `{"file_path":"main.go"}`, Cwd: "/requested"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if text, ok := got.String(); !ok || text != "ok" || roots.root != "/requested" || invoker.root != "/workspace" || invoker.name != "read" || invoker.arguments != `{"file_path":"main.go"}` {
		t.Fatalf("result=%#v cwd=%q root=%q name=%q arguments=%q", got, roots.root, invoker.root, invoker.name, invoker.arguments)
	}
}
