package agentmemory

import (
	"context"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// SearchStore supplies the active memory corpus consumed by semantic search.
type SearchStore interface {
	ItemsForSearch(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
}

// Embedder supplies an optional semantic signal. Embedding remains best-effort:
// keyword ranking is still useful when the model is absent or unhealthy.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	ID() string
}

// Searcher coordinates corpus I/O and optional embedding, then delegates pure
// ranking to the agent-memory domain.
type Searcher struct {
	store           SearchStore
	resolveEmbedder func(context.Context) (Embedder, error)
}

// NewSearcher constructs the search use case. A nil resolver selects
// keyword-only search.
func NewSearcher(store SearchStore, resolveEmbedder func(context.Context) (Embedder, error)) *Searcher {
	return &Searcher{store: store, resolveEmbedder: resolveEmbedder}
}

// Search returns up to topK relevant memory items.
func (s *Searcher) Search(ctx context.Context, scope domain.Scope, project, query string, topK int) ([]domain.Item, error) {
	if s == nil || s.store == nil || topK <= 0 {
		return nil, nil
	}
	items, err := s.store.ItemsForSearch(ctx, scope, project)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	var queryVector []float32
	if s.resolveEmbedder != nil {
		if embedder, resolveErr := s.resolveEmbedder(ctx); resolveErr == nil && embedder != nil {
			if vectors, embedErr := embedder.Embed(ctx, []string{query}); embedErr == nil && len(vectors) == 1 {
				queryVector = vectors[0]
			}
		}
	}
	return domain.Rank(query, queryVector, items, topK), nil
}
