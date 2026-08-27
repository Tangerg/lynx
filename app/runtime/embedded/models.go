package embedded

import (
	"context"

	"github.com/Tangerg/scope/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

// ListModels returns models available through configured providers.
func (r *Runtime) ListModels(ctx context.Context, request protocol.ListModelsRequest, options CallOptions) (*protocol.Page[protocol.Model], error) {
	return r.invoke[protocol.ListModelsRequest, *protocol.Page[protocol.Model]](ctx, operation.ModelsList, request, callOptions(options))
}

// GetUtilityRole returns the model used for maintenance work.
func (r *Runtime) GetUtilityRole(ctx context.Context, options CallOptions) (*protocol.UtilityRole, error) {
	return r.invoke[struct{}, *protocol.UtilityRole](ctx, operation.ModelsGetUtilityRole, struct{}{}, callOptions(options))
}

// SetUtilityRole changes the model used for maintenance work.
func (r *Runtime) SetUtilityRole(ctx context.Context, request protocol.UtilityRole, options CommandOptions) (*protocol.UtilityRole, error) {
	return r.invoke[protocol.UtilityRole, *protocol.UtilityRole](ctx, operation.ModelsSetUtilityRole, request, commandOptions(options))
}

// GetEmbeddingRole returns the model used for embeddings.
func (r *Runtime) GetEmbeddingRole(ctx context.Context, options CallOptions) (*protocol.EmbeddingRole, error) {
	return r.invoke[struct{}, *protocol.EmbeddingRole](ctx, operation.ModelsGetEmbeddingRole, struct{}{}, callOptions(options))
}

// SetEmbeddingRole changes the model used for embeddings.
func (r *Runtime) SetEmbeddingRole(ctx context.Context, request protocol.EmbeddingRole, options CommandOptions) (*protocol.EmbeddingRole, error) {
	return r.invoke[protocol.EmbeddingRole, *protocol.EmbeddingRole](ctx, operation.ModelsSetEmbeddingRole, request, commandOptions(options))
}
