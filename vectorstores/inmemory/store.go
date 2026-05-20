// Package inmemory is an in-process [vectorstore.Store] backed by a
// `map[string]record` plus a configurable similarity function. It is
// intended for demos, unit tests, and corpora that fit in RAM.
//
// Concurrency: every public method is safe for concurrent use; reads
// take an RLock, writes take a Lock. The store performs no I/O —
// errors come from the embedding client or the filter parser.
//
// Persistence is out of scope: the store has no durability story.
package inmemory

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

// StoreConfig configures a [Store].
type StoreConfig struct {
	// EmbeddingClient embeds documents on Create and queries on
	// Retrieve. Required.
	EmbeddingClient *embedding.Client

	// Similarity is the function used to score retrieved documents
	// against the query embedding. Optional; defaults to
	// [CosineSimilarity]. Implementations must return higher-is-more-
	// similar.
	Similarity Similarity
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.EmbeddingClient == nil {
		return ErrMissingEmbeddingClient
	}
	if c.Similarity == nil {
		c.Similarity = CosineSimilarity
	}
	return nil
}

// record pairs a stored document with the embedding vector that was
// computed for it at Create time. Re-embedding never happens for
// existing records — the cost of a fresh vectorisation is paid once.
type record struct {
	doc       *document.Document
	embedding []float64
}

var _ vectorstore.Store = (*Store)(nil)

// Store is the in-memory [vectorstore.Store] implementation.
// Construct with [NewStore].
type Store struct {
	embedder   *embedding.Client
	similarity Similarity

	mu      sync.RWMutex
	records map[string]record
}


func NewStore(cfg *StoreConfig) (*Store, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Store{
		embedder:   cfg.EmbeddingClient,
		similarity: cfg.Similarity,
		records:    map[string]record{},
	}, nil
}


func (s *Store) Metadata() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		Provider:     Provider,
		NativeClient: s,
	}
}

// Len reports the number of stored records — exposed for tests /
// monitoring; not part of the [vectorstore.Store] contract.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// Create embeds the request documents and indexes them by ID. Each
// document must have a non-empty ID (use [document.Document.ID] or
// assign one before calling). Existing IDs are overwritten — this
// mirrors the upsert semantics most vendor stores expose.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("inmemory.Store.Create: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "inmemory", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	texts := make([]string, 0, len(req.Documents))
	for i, doc := range req.Documents {
		if doc == nil {
			return fmt.Errorf("inmemory.Store.Create: document[%d] is nil", i)
		}
		if doc.ID == "" {
			return fmt.Errorf("inmemory.Store.Create: document[%d] has empty ID", i)
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
		return fmt.Errorf("inmemory.Store.Create: embed: %w", err)
	}
	if len(embeddings) != len(req.Documents) {
		return fmt.Errorf("inmemory.Store.Create: embedder returned %d vectors for %d documents",
			len(embeddings), len(req.Documents))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, doc := range req.Documents {
		s.records[doc.ID] = record{doc: doc, embedding: embeddings[i]}
	}
	return nil
}

// Retrieve embeds the query, scores every record by similarity, and
// returns the top-K above MinScore. Filtering happens BEFORE scoring
// to keep the cost O(filtered × dim) rather than O(all × dim).
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (out []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("inmemory.Store.Retrieve: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "inmemory", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(out)) }()

	var query []float64
	query, _, err = s.embedder.
		Embed().
		WithTexts([]string{req.Query}).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("inmemory.Store.Retrieve: embed query: %w", err)
	}

	type scored struct {
		doc   *document.Document
		score float64
	}

	s.mu.RLock()
	candidates := make([]scored, 0, len(s.records))
	for _, rec := range s.records {
		if req.Filter != nil {
			match, ferr := matchesFilter(req.Filter, rec.doc.Metadata)
			if ferr != nil {
				s.mu.RUnlock()
				return nil, fmt.Errorf("inmemory.Store.Retrieve: filter: %w", ferr)
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
	out = make([]*document.Document, 0, limit)
	for i := range limit {
		out = append(out, candidates[i].doc)
	}
	return out, nil
}

// Delete removes every record whose metadata matches the filter
// expression. The number of records actually removed is not reported
// by the [vectorstore.Deleter] contract; call [Store.Len] before and
// after if you need the delta.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("inmemory.Store.Delete: %w", err)
	}

	_, span := tracing.StartDelete(ctx, "inmemory")
	defer func() { tracing.Finish(span, err) }()

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.records {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("inmemory.Store.Delete: %w", err)
		}
		match, err := matchesFilter(req.Filter, rec.doc.Metadata)
		if err != nil {
			return fmt.Errorf("inmemory.Store.Delete: filter: %w", err)
		}
		if match {
			delete(s.records, id)
		}
	}
	return nil
}

// Clear removes every record. Useful for test setup/teardown; not
// part of the [vectorstore.Store] interface.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.records)
}
