package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListKnowledge returns the effective knowledge cascade for a workspace.
func (r *Runtime) ListKnowledge(ctx context.Context, request protocol.WorkspaceQuery, options CallOptions) (*protocol.Page[protocol.KnowledgeEntry], error) {
	return r.invoke[protocol.WorkspaceQuery, *protocol.Page[protocol.KnowledgeEntry]](ctx, operation.KnowledgeList, request, callOptions(options))
}

// GetKnowledge returns one knowledge entry.
func (r *Runtime) GetKnowledge(ctx context.Context, request protocol.GetKnowledgeRequest, options CallOptions) (*protocol.KnowledgeEntry, error) {
	return r.invoke[protocol.GetKnowledgeRequest, *protocol.KnowledgeEntry](ctx, operation.KnowledgeGet, request, callOptions(options))
}

// UpdateKnowledge conditionally replaces one user-editable knowledge entry.
func (r *Runtime) UpdateKnowledge(ctx context.Context, request protocol.UpdateKnowledgeRequest, options CommandOptions) (*protocol.KnowledgeEntry, error) {
	return r.invoke[protocol.UpdateKnowledgeRequest, *protocol.KnowledgeEntry](ctx, operation.KnowledgeUpdate, request, commandOptions(options))
}
