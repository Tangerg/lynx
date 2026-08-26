package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListTools returns the Runtime's model-facing tool descriptors.
func (r *Runtime) ListTools(ctx context.Context, options CallOptions) (*protocol.Page[protocol.ToolSpec], error) {
	return r.invoke[struct{}, *protocol.Page[protocol.ToolSpec]](ctx, operation.ToolsList, struct{}{}, callOptions(options))
}

// InvokeTool invokes one Runtime tool outside an Agent Run.
func (r *Runtime) InvokeTool(ctx context.Context, request protocol.InvokeToolRequest, options CommandOptions) (any, error) {
	return r.invoke[protocol.InvokeToolRequest, any](ctx, operation.ToolsInvoke, request, commandOptions(options))
}
