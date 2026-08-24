package agentmemory

import (
	"context"

	domain "github.com/Tangerg/lynx/app/runtime/internal/domain/agentmemory"
)

// SearchStore supplies the active memory corpus and owns its derived embedding
// cache. Cache writes are conditional on the exact content digest, so a late
// model response cannot overwrite an item edited while embedding was in flight.
type SearchStore interface {
	ItemsForSearch(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	SetEmbeddings(ctx context.Context, updates []domain.EmbeddingUpdate) error
}

// Embedder supplies an optional semantic signal. Embedding remains best-effort:
// keyword ranking is still useful when the model is absent or unhealthy.
type Embedder interface {
	// ID identifies the exact non-secret client configuration that selects the
	// vector coordinate system, including any custom endpoint identity.
	ID() string
	Embed(ctx context.Context, texts []string) ([][]float32, error)
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
	if s.resolveEmbedder == nil {
		return domain.Rank(query, nil, items, topK), nil
	}
	embedder, err := s.resolveEmbedder(ctx)
	if err != nil || embedder == nil {
		return domain.Rank(query, nil, items, topK), nil
	}
	space := embedder.ID()
	if space == "" {
		return domain.Rank(query, nil, items, topK), nil
	}
	queryVectors, err := embedder.Embed(ctx, []string{query})
	if err != nil || len(queryVectors) != 1 || !usableVector(queryVectors[0], 0) {
		return domain.Rank(query, nil, items, topK), nil
	}
	queryVector := queryVectors[0]

	stale := make([]int, 0, len(items))
	texts := make([]string, 0, len(items))
	for index := range items {
		if items[index].EmbeddingSpace == space && usableVector(items[index].Embedding, len(queryVector)) {
			continue
		}
		items[index].EmbeddingSpace = ""
		items[index].Embedding = nil
		stale = append(stale, index)
		texts = append(texts, items[index].Content)
	}
	if len(stale) != 0 {
		vectors, embedErr := embedder.Embed(ctx, texts)
		if embedErr == nil && len(vectors) == len(stale) {
			updates := make([]domain.EmbeddingUpdate, 0, len(stale))
			for offset, index := range stale {
				if !usableVector(vectors[offset], len(queryVector)) {
					updates = nil
					break
				}
				update, updateErr := domain.NewEmbeddingUpdate(items[index], space, vectors[offset])
				if updateErr != nil {
					updates = nil
					break
				}
				updates = append(updates, update)
			}
			if len(updates) == len(stale) {
				for offset, index := range stale {
					items[index].EmbeddingSpace = updates[offset].Space
					items[index].Embedding = updates[offset].Vector
				}
				// The cache is derived state: the current request already owns exact
				// vectors, so a failed or losing conditional write must not turn a
				// useful search into an application failure.
				_ = s.store.SetEmbeddings(ctx, updates)
			}
		}
	}
	return domain.Rank(query, queryVector, items, topK), nil
}

func usableVector(vector []float32, dimension int) bool {
	if len(vector) == 0 || dimension > 0 && len(vector) != dimension {
		return false
	}
	return domain.ValidateEmbeddingVector(vector) == nil
}
