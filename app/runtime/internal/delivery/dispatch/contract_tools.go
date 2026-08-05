package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerTools(r *Registry) {
	Query(r, MethodMeta{Name: "tools.list", Stability: stable},
		func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return d.api.ListTools(ctx)
		})

	Command(r, MethodMeta{
		Name: "tools.invoke",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
		return d.api.InvokeTool(ctx, in)
	})
}
