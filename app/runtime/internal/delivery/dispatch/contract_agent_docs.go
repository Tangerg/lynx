package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerAgentDocs(registry *Registry) {
	Query(registry, MethodMeta{
		Name:      "agentDocs.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return router.api.ListAgentDocs(ctx, request)
	})
}
