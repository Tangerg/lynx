package capabilities

import (
	"context"

	toolsvc "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ListRegisteredTools returns every tool the runtime exposes for direct
// diagnostic invocation.
func (c *Coordinator) ListRegisteredTools(ctx context.Context) ([]toolsvc.Tool, error) {
	return c.tools.List(ctx)
}

// InvokeRegisteredTool runs one registered tool directly outside a chat turn.
func (c *Coordinator) InvokeRegisteredTool(ctx context.Context, name string, arguments string) (string, error) {
	return c.tools.Invoke(ctx, name, arguments)
}
