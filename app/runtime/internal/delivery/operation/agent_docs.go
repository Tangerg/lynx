package operation

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

const AgentDocsList Name = "agentDocs.list"

func registerAgentDocs(registry *Registry) {
	registry.Query(MethodMeta{
		Name:   AgentDocsList,
		Errors: []string{protocol.ErrWorkspaceUnavailable.Error()},
	}, func(service interface {
		ListAgentDocs(context.Context, protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error)
	}, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return service.ListAgentDocs(ctx, request)
	})
}
