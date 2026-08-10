package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// SearchCodebase searches the semantic codebase index.
func (r *Runtime) SearchCodebase(ctx context.Context, request protocol.CodebaseSearchRequest, options CallOptions) (*protocol.CodebaseSearchResult, error) {
	return invoke[protocol.CodebaseSearchRequest, *protocol.CodebaseSearchResult](ctx, r, "codebase.search", request, callOptions(options))
}

// GetCodebaseStatus returns index status for a workspace.
func (r *Runtime) GetCodebaseStatus(ctx context.Context, request protocol.CodebaseStatusRequest, options CallOptions) (*protocol.CodebaseStatus, error) {
	return invoke[protocol.CodebaseStatusRequest, *protocol.CodebaseStatus](ctx, r, "codebase.status", request, callOptions(options))
}

// ReindexCodebase requests a fresh workspace index.
func (r *Runtime) ReindexCodebase(ctx context.Context, request protocol.CodebaseReindexRequest, options CommandOptions) (*protocol.CodebaseReindexResponse, error) {
	return invoke[protocol.CodebaseReindexRequest, *protocol.CodebaseReindexResponse](ctx, r, "codebase.reindex", request, commandOptions(options))
}
