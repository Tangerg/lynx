package web

import (
	"context"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type readOnlyTool struct {
	inner toolcontract.Tool
}

func (t *readOnlyTool) bind[I, O any](config toolcontract.FuncConfig, call func(context.Context, I) (O, error)) error {
	inner, err := toolcontract.NewFunc(config, call)
	if err != nil {
		return err
	}
	t.inner = inner
	return nil
}

func (t readOnlyTool) Definition() chat.ToolDefinition { return t.inner.Definition() }

func (t readOnlyTool) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	return t.inner.Call(ctx, invocation)
}

// Network reads have no local resource conflict, so independent calls may run
// concurrently under the tool executor's optional scheduling contract.
func (readOnlyTool) ConcurrencyKey(toolcontract.Invocation) (key string, concurrent bool) {
	return "", true
}
