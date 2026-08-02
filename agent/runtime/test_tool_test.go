package runtime_test

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

type runtimeTestTool struct {
	name string
	call func(context.Context) (string, error)
}

func (t *runtimeTestTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        t.name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (t *runtimeTestTool) Call(ctx context.Context, _ string) (string, error) {
	if t.call == nil {
		return "", nil
	}
	return t.call(ctx)
}

func newRuntimeTestTool(name string, call func(context.Context) (string, error)) tool.Tool {
	return &runtimeTestTool{name: name, call: call}
}
