package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListAgentDocs returns agent instruction documents applicable to a workspace.
func (r *Runtime) ListAgentDocs(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.AgentDoc]](ctx, operation.AgentDocsList, request, callOptions(options))
}
