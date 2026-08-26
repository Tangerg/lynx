package modelclient

import (
	"context"

	"github.com/Tangerg/lynx/core/chatclient"

	agentmemoryapp "github.com/Tangerg/lynx/app/runtime/internal/application/agentmemory"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

// RoleSource is the read view a live specialized-model resolver needs. The
// source's owner decides how role changes are synchronized.
type RoleSource interface {
	Role() modelref.Selection
}

// UtilityClient returns the current specialized utility client, falling back to
// main when no role is configured or the configured client cannot be resolved.
func (r *ChatResolver) UtilityClient(main *chatclient.Client, roles RoleSource) func(context.Context) *chatclient.Client {
	return func(ctx context.Context) *chatclient.Client {
		if r == nil || roles == nil {
			return main
		}
		role := roles.Role()
		if !role.Configured() {
			return main
		}
		client, err := r.ResolveChat(ctx, role)
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

// ResolveMemory returns the optional embedder configured for agent-memory
// ranking. An absent role is a normal keyword-only configuration.
func (r *RoleEmbedder) ResolveMemory(ctx context.Context) (agentmemoryapp.Embedder, error) {
	if r == nil || r.resolver == nil || r.roles == nil {
		return nil, nil
	}
	role := r.roles.Role()
	if !role.Configured() {
		return nil, nil
	}
	return r.resolver.Resolve(ctx, role)
}
