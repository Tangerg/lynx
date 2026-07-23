package modelclient

import (
	"context"

	"github.com/Tangerg/lynx/chatclient"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/codebaseindex"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelrole"
)

// RoleSource is the read view a live specialized-model resolver needs. The
// source's owner decides how role changes are synchronized.
type RoleSource interface {
	Role() modelrole.Role
}

// UtilityClient returns the current specialized utility client, falling back to
// main when no role is configured or the configured client cannot be resolved.
func (r *ClientResolver) UtilityClient(main *chatclient.Client, roles RoleSource) func(context.Context) *chatclient.Client {
	return func(ctx context.Context) *chatclient.Client {
		if r == nil || roles == nil {
			return main
		}
		role := roles.Role()
		if !role.Configured() {
			return main
		}
		client, err := r.ResolveClient(ctx, role.ProviderID(), role.Model())
		if err != nil || client == nil {
			return main
		}
		return client
	}
}

// RoleEmbedder resolves the live embedding role through an embedding resolver.
type RoleEmbedder struct {
	resolver *EmbeddingResolver
	roles    RoleSource
}

// NewRoleEmbedder builds a live embedding-role resolver.
func NewRoleEmbedder(resolver *EmbeddingResolver, roles RoleSource) *RoleEmbedder {
	return &RoleEmbedder{resolver: resolver, roles: roles}
}

// Resolve returns the embedder configured for the current role.
func (r *RoleEmbedder) Resolve(ctx context.Context) (codebaseindex.Embedder, error) {
	if r == nil || r.resolver == nil || r.roles == nil {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, codebaseindex.ErrNoEmbeddingModel
	}
	return r.resolver.Resolve(ctx, role.ProviderID(), role.Model())
}

// ResolveMemory adapts the same live embedder to agent-memory search.
func (r *RoleEmbedder) ResolveMemory(ctx context.Context) (agentmemory.Embedder, error) {
	return r.Resolve(ctx)
}
