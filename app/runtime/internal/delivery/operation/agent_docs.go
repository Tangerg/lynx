package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerAgentDocs(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "agentDocs.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(service Service, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return service.ListAgentDocs(ctx, request)
	})
}
