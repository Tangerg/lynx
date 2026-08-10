package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerTools(registry *Registry) {
	Query(registry, MethodMeta{Name: "tools.list", Stability: stable},
		func(router *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return router.api.ListTools(ctx)
		})

	Command(registry, MethodMeta{
		Name: "tools.invoke",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.InvokeToolRequest) (any, error) {
		return router.api.InvokeTool(ctx, request)
	})
}
