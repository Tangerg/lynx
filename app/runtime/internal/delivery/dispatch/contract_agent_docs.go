package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerAgentDocs(r *Registry) {
	Query(r, MethodMeta{
		Name:      "agentDocs.list",
		Errors:    []string{protocol.ErrWorkspaceUnavailable.Error()},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.WorkspaceQuery) (*protocol.Page[protocol.AgentDoc], error) {
		return d.api.ListAgentDocs(ctx, in)
	})
}
