package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListKnowledge returns the effective knowledge cascade for a workspace.
func (r *Runtime) ListKnowledge(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error) {
	return invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.KnowledgeEntry]](ctx, r, "knowledge.list", request, callOptions(options))
}

// GetKnowledge returns one knowledge entry.
func (r *Runtime) GetKnowledge(ctx context.Context, request protocol.GetKnowledgeRequest, options CallOptions) (*protocol.KnowledgeEntry, error) {
	return invoke[protocol.GetKnowledgeRequest, *protocol.KnowledgeEntry](ctx, r, "knowledge.get", request, callOptions(options))
}

// UpdateKnowledge replaces one user-editable knowledge entry.
func (r *Runtime) UpdateKnowledge(ctx context.Context, request protocol.UpdateKnowledgeRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "knowledge.update", request, commandOptions(options))
}
