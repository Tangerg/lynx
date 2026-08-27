package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const (
	ToolsList   Name = "tools.list"
	ToolsInvoke Name = "tools.invoke"
)

func registerTools(registry *Registry) {
	registry.Query(MethodMeta{Name: ToolsList},
		func(service interface {
			ListTools(context.Context) (*protocol.Page[protocol.ToolSpec], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return service.ListTools(ctx)
		})

	registry.Command(MethodMeta{
		Name: ToolsInvoke,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		InvokeTool(context.Context, protocol.InvokeToolRequest) (any, error)
	}, ctx context.Context, request protocol.InvokeToolRequest) (any, error) {
		return service.InvokeTool(ctx, request)
	})
}
