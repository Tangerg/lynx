package inmemory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/embeddingclient"
	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

// StoreConfig fixes the two policies an in-memory store cannot infer per call:
// the required embedding model and the score function shared by indexing and
// retrieval. A nil Similarity selects CosineSimilarity; the model has no safe
// default because it determines vector shape and meaning.
type StoreConfig struct {
	// EmbeddingModel embeds documents on Index and queries on Search.
	// Required.
	EmbeddingModel embedding.Model

	// Similarity is the function used to score retrieved documents
	// against the query embedding. Optional; defaults to
	// [CosineSimilarity]. Implementations must return higher-is-more-
	// similar.
	Similarity Similarity
}

func (s *StoreConfig) applyDefaults() {
	if s.Similarity == nil {
		s.Similarity = CosineSimilarity
	}
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.EmbeddingModel == nil {
		return ErrMissingEmbeddingModel
	}
	return nil
}

// record pairs a stored document with the embedding vector that was
// computed for it at Index time. Re-embedding never happens for
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

// Store is the concurrency-safe reference implementation of the vector-store
// capability contracts. Index snapshots documents, embeds each upsert once,
// and replaces records by caller-owned ID. Search snapshots results, evaluates
// the same filter AST exposed to external backends, and orders normalized scores
// deterministically. Deletes never expose the internal record map.
type Store struct {
	embeddingClient embeddingclient.Client
	similarity      Similarity

	mu      sync.RWMutex
	records map[string]record
}

func NewStore(config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if errors.Is(err, embeddingclient.ErrNilModel) {
		return nil, ErrMissingEmbeddingModel
	}
	if err != nil {
		return nil, fmt.Errorf("inmemory: create store: create embedding client: %w", err)
	}
	return &Store{
		embeddingClient: embeddingClient,
		similarity:      config.Similarity,
		records:         map[string]record{},
	}, nil
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return fmt.Errorf("inmemory: index documents: %w", validateErr)
	}
	docs := request.Documents

	texts := make([]string, 0, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return fmt.Errorf("inmemory: index documents: document[%d] is nil", i)
		}
		if doc.ID == "" {
			return fmt.Errorf("inmemory: index documents: document[%d] has empty ID", i)
		}
		texts = append(texts, doc.Text)
	}

	var embeddings [][]float64
	embeddings, err = s.embeddingClient.EmbedTexts(ctx, texts)
	if err != nil {
		return fmt.Errorf("inmemory: index documents: embed: %w", err)
	}
	if len(embeddings) != len(docs) {
		return fmt.Errorf("inmemory: index documents: embedder returned %d vectors for %d documents",
			len(embeddings), len(docs))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, doc := range docs {
		s.records[doc.ID] = record{doc: doc, embedding: embeddings[i]}
	}
	return nil
}

func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var out []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("inmemory: search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var query []float64
	query, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("inmemory: search: embed query: %w", err)
	}

	type scored struct {
		doc   *document.Document
		score vectorstore.Score
	}

	s.mu.RLock()
	candidates := make([]scored, 0, len(s.records))
	for _, rec := range s.records {
		if req.Options.Filter != nil {
			metadataValues, decodeErr := rec.doc.Metadata.Values()
			if decodeErr != nil {
				s.mu.RUnlock()
				return nil, fmt.Errorf("inmemory: search: metadata: %w", decodeErr)
			}
			match, ferr := matchesFilter(req.Options.Filter, metadataValues)
			if ferr != nil {
				s.mu.RUnlock()
				return nil, fmt.Errorf("inmemory: search: filter: %w", ferr)
			}
			if !match {
				continue
			}
		}
		score := s.similarity(query, rec.embedding)
		if score < req.Options.MinScore {
			continue
		}
		candidates = append(candidates, scored{doc: rec.doc, score: score})
	}
	s.mu.RUnlock()

	slices.SortStableFunc(candidates, func(a, b scored) int {
		return cmp.Compare(b.score, a.score)
	})

	limit := min(req.Options.TopK, len(candidates))
	out = make([]*vectorstore.SearchResult, 0, limit)
	for i := range limit {
		out = append(out, &vectorstore.SearchResult{Document: candidates[i].doc, Score: candidates[i].score})
	}
	return &vectorstore.SearchResponse{Results: out}, nil
}

// Delete removes every record whose metadata matches the filter
// expression. The number of records actually removed is not reported
// by the [vectorstore.FilterDeleter] contract; call [Store.Len] before and
// after if you need the delta.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("inmemory: delete by filter: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.records {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("inmemory: delete by filter: %w", err)
		}
		metadataValues, err := rec.doc.Metadata.Values()
		if err != nil {
			return fmt.Errorf("inmemory: delete by filter: metadata: %w", err)
		}
		match, err := matchesFilter(expr, metadataValues)
		if err != nil {
			return fmt.Errorf("inmemory: delete by filter: filter: %w", err)
		}
		if match {
			delete(s.records, id)
		}
	}
	return nil
}

func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("inmemory: delete by ID: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.records, id)
	}
	return nil
}

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.records)
}
