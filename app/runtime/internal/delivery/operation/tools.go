package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerTools(registry *Registry) {
	Query(registry, MethodMeta{Name: "tools.list", Stability: stable},
		func(service interface {
			ListTools(context.Context) (*protocol.Page[protocol.ToolSpec], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return service.ListTools(ctx)
		})

	Command(registry, MethodMeta{
		Name: "tools.invoke",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
		Stability: stable,
	}, func(service interface {
		InvokeTool(context.Context, protocol.InvokeToolRequest) (any, error)
	}, ctx context.Context, request protocol.InvokeToolRequest) (any, error) {
		return service.InvokeTool(ctx, request)
	})
}
