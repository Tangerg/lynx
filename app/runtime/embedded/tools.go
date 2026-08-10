package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListTools returns the Runtime's model-facing tool descriptors.
func (r *Runtime) ListTools(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	return invoke[struct{}, *protocol.Page[protocol.ToolSpec]](ctx, r, "tools.list", struct{}{}, callOptions(options))
}

// InvokeTool invokes one Runtime tool outside an Agent Run.
func (r *Runtime) InvokeTool(ctx context.Context, request protocol.InvokeToolRequest, options CommandOptions) (any, error) {
	return invoke[protocol.InvokeToolRequest, any](ctx, r, "tools.invoke", request, commandOptions(options))
}
