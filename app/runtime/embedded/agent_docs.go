package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListAgentDocs returns agent instruction documents applicable to a workspace.
func (r *Runtime) ListAgentDocs(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.AgentDoc], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.AgentDoc]](ctx, r, "agentDocs.list", request, callOptions(options))
}
