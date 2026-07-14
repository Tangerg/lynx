package inmemory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

// StoreConfig configures a [Store].
type StoreConfig struct {
	// EmbeddingClient embeds documents on Add and queries on
	// Search. Required.
	EmbeddingClient *embedding.Client

	// Similarity is the function used to score retrieved documents
	// against the query embedding. Optional; defaults to
	// [CosineSimilarity]. Implementations must return higher-is-more-
	// similar.
	Similarity Similarity
}

func (c *StoreConfig) ApplyDefaults() {
	if c.Similarity == nil {
		c.Similarity = CosineSimilarity
	}
}

func (c *StoreConfig) Validate() error {
	if c.EmbeddingClient == nil {
		return ErrMissingEmbeddingClient
	}
	return nil
}

// record pairs a stored document with the embedding vector that was
// computed for it at Add time. Re-embedding never happens for
// existing records — the cost of a fresh vectorisation is paid once.
type record struct {
	doc       *document.Document
	embedding []float64
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store is the in-memory the vectorstore capability interfaces implementation.
// Construct with [NewStore].
type Store struct {
	embedder   *embedding.Client
	similarity Similarity

	mu      sync.RWMutex
	records map[string]record
}

func NewStore(cfg StoreConfig) (*Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Store{
		embedder:   cfg.EmbeddingClient,
		similarity: cfg.Similarity,
		records:    map[string]record{},
	}, nil
}

// Len reports the number of stored records — exposed for tests /
// monitoring; not part of the the vectorstore capability interfaces contract.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// Add embeds the documents and indexes them by ID. Each
// document must have a non-empty ID (use [document.Document.ID] or
// assign one before calling). Existing IDs are overwritten — this
// mirrors the upsert semantics most vendor stores expose.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "inmemory", len(docs))
	defer func() { tracing.Finish(span, err) }()

	texts := make([]string, 0, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return fmt.Errorf("inmemory.Store.Add: document[%d] is nil", i)
		}
		if doc.ID == "" {
			return fmt.Errorf("inmemory.Store.Add: document[%d] has empty ID", i)
		}
		texts = append(texts, doc.Text)
	}

	var embeddings [][]float64
	embeddings, _, err = s.embedder.
		Embed().
		WithTexts(texts).
		Call().
		Embeddings(ctx)
	if err != nil {
		return fmt.Errorf("inmemory.Store.Add: embed: %w", err)
	}
	if len(embeddings) != len(docs) {
		return fmt.Errorf("inmemory.Store.Add: embedder returned %d vectors for %d documents",
			len(embeddings), len(docs))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, doc := range docs {
		s.records[doc.ID] = record{doc: doc, embedding: embeddings[i]}
	}
	return nil
}

// Search embeds the query, scores every record by similarity, and
// returns the top-K above MinScore. Filtering happens BEFORE scoring
// to keep the cost O(filtered × dim) rather than O(all × dim).
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (out []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("inmemory.Store.Search: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "inmemory", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(out)) }()

	var query []float64
	query, _, err = s.embedder.
		Embed().
		WithTexts([]string{req.Query}).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("inmemory.Store.Search: embed query: %w", err)
	}

	type scored struct {
		doc   *document.Document
		score float64
	}

	s.mu.RLock()
	candidates := make([]scored, 0, len(s.records))
	for _, rec := range s.records {
		if req.Filter != nil {
			metadataValues, decodeErr := rec.doc.Metadata.Values()
			if decodeErr != nil {
				s.mu.RUnlock()
				return nil, fmt.Errorf("inmemory.Store.Search: metadata: %w", decodeErr)
			}
			match, ferr := matchesFilter(req.Filter, metadataValues)
			if ferr != nil {
				s.mu.RUnlock()
				return nil, fmt.Errorf("inmemory.Store.Search: filter: %w", ferr)
			}
			if !match {
				continue
			}
		}
		score := s.similarity(query, rec.embedding)
		if score < req.MinScore {
			continue
		}
		candidates = append(candidates, scored{doc: rec.doc, score: score})
	}
	s.mu.RUnlock()

	slices.SortStableFunc(candidates, func(a, b scored) int {
		return cmp.Compare(b.score, a.score)
	})

	limit := min(req.TopK, len(candidates))
	out = make([]vectorstore.Match, 0, limit)
	for i := range limit {
		out = append(out, vectorstore.Match{Document: candidates[i].doc, Score: candidates[i].score})
	}
	return out, nil
}

// Delete removes every record whose metadata matches the filter
// expression. The number of records actually removed is not reported
// by the [vectorstore.FilterDeleter] contract; call [Store.Len] before and
// after if you need the delta.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	_, span := tracing.StartDelete(ctx, "inmemory")
	defer func() { tracing.Finish(span, err) }()

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.records {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("inmemory.Store.Delete: %w", err)
		}
		metadataValues, err := rec.doc.Metadata.Values()
		if err != nil {
			return fmt.Errorf("inmemory.Store.Delete: metadata: %w", err)
		}
		match, err := matchesFilter(expr, metadataValues)
		if err != nil {
			return fmt.Errorf("inmemory.Store.Delete: filter: %w", err)
		}
		if match {
			delete(s.records, id)
		}
	}
	return nil
}

// DeleteIDs removes the records with the given ids. An empty slice is
// a no-op; unknown ids are ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("inmemory.Store.DeleteIDs: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	_, span := tracing.StartDelete(ctx, "inmemory")
	defer func() { tracing.Finish(span, err) }()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.records, id)
	}
	return nil
}

// Clear removes every record. Useful for test setup/teardown; not
// part of the the vectorstore capability interfaces interface.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.records)
}
