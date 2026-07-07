package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ListRegisteredTools returns every tool the runtime exposes for direct
// diagnostic invocation.
func (r *Runtime) ListRegisteredTools(ctx context.Context) ([]tool.Tool, error) {
	return r.tools.List(ctx)
}

// InvokeRegisteredTool runs one registered tool directly outside a chat turn.
func (r *Runtime) InvokeRegisteredTool(ctx context.Context, name string, arguments string) (string, error) {
	return r.tools.Invoke(ctx, name, arguments)
}
