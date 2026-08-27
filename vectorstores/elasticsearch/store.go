package elasticsearch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/embeddingclient"
	"github.com/Tangerg/scope/core/vectorstore"
)

const Provider = "Elasticsearch"

const (
	DefaultIndexName        = "scope-vector-index"
	DefaultEmbeddingField   = "embedding"
	DefaultContentField     = "content"
	DefaultMetadataField    = "metadata"
	DefaultSimilarity       = SimilarityCosine
	defaultNumCandidatesMul = 1.5 // num_candidates = ceil(topK * multiplier)
)

// SimilarityFunction selects the Elasticsearch dense-vector similarity
// metric. The chosen value is recorded in the index mapping; changing
// it after the index is created has no effect.
type SimilarityFunction string

const (
	// SimilarityCosine — cosine similarity. Default; suitable for
	// most use cases.
	SimilarityCosine SimilarityFunction = "cosine"

	// SimilarityL2 — Euclidean (L2) distance.
	SimilarityL2 SimilarityFunction = "l2_norm"

	// SimilarityDotProduct — dot product. Recommended for
	// already-normalized embeddings (e.g. OpenAI's).
	SimilarityDotProduct SimilarityFunction = "dot_product"
)

func (s SimilarityFunction) Valid() bool {
	switch s {
	case SimilarityCosine, SimilarityL2, SimilarityDotProduct:
		return true
	default:
		return false
	}
}

func (s SimilarityFunction) String() string { return string(s) }

// StoreConfig contains configuration options for the Elasticsearch
// vector store.
type StoreConfig struct {
	// Client is the go-elasticsearch typed client. Required.
	Client *elasticsearch.Client

	// IndexName names the Elasticsearch index. Optional: defaults
	// to [DefaultIndexName].
	IndexName string

	// EmbeddingField is the dense_vector field name. Optional:
	// defaults to [DefaultEmbeddingField].
	EmbeddingField string

	// ContentField is the field that stores the document text.
	// Optional: defaults to [DefaultContentField].
	ContentField string

	// MetadataField is the object field that stores metadata.
	// Optional: defaults to [DefaultMetadataField]. Set to "" to
	// flatten metadata onto the document root (filters then
	// reference bare field names).
	MetadataField string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before bulk upsert. Required.
	DocumentBatcher vectorstore.Batcher

	// Dimensions sets the dense_vector width for a newly created index. When
	// zero, the store probes EmbeddingModel only if it must create the index.
	Dimensions int

	// Similarity selects the similarity metric used at index time.
	// Optional: defaults to [SimilarityCosine].
	Similarity SimilarityFunction

	// InitializeSchema, when true, creates the index with the right
	// mapping if it doesn't already exist. When false and the index
	// is missing, [NewStore] returns [ErrIndexMissing].
	InitializeSchema bool

	// NumCandidatesMultiplier scales the KNN num_candidates parameter.
	// num_candidates = ceil(topK * multiplier). Higher = better
	// recall, slower. Optional: defaults to 1.5.
	NumCandidatesMultiplier float64
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Client == nil {
		return errors.New("elasticsearch: Client is required")
	}
	if s.EmbeddingModel == nil {
		return errors.New("elasticsearch: EmbeddingModel is required")
	}
	if s.DocumentBatcher == nil {
		return errors.New("elasticsearch: DocumentBatcher is required")
	}
	if s.Dimensions < 0 {
		return errors.New("elasticsearch: Dimensions must be >= 0")
	}
	if !s.Similarity.Valid() {
		return fmt.Errorf("elasticsearch: unsupported Similarity %q", s.Similarity)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (s *StoreConfig) applyDefaults() {
	s.IndexName = cmp.Or(s.IndexName, DefaultIndexName)
	s.EmbeddingField = cmp.Or(s.EmbeddingField, DefaultEmbeddingField)
	s.ContentField = cmp.Or(s.ContentField, DefaultContentField)
	if s.MetadataField == "" {
		s.MetadataField = DefaultMetadataField
	}
	s.Similarity = cmp.Or(s.Similarity, DefaultSimilarity)
	if s.NumCandidatesMultiplier <= 0 {
		s.NumCandidatesMultiplier = defaultNumCandidatesMul
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store is an Elasticsearch-backed implementation of
// the vectorstore capability interfaces. It uses the dense_vector field type and the
// `knn` query for similarity search.
type Store struct {
	client           *elasticsearch.Client
	indexName        string
	embeddingField   string
	contentField     string
	metadataField    string
	embeddingClient  embeddingclient.Client
	documentBatcher  vectorstore.Batcher
	dimensions       int
	similarity       SimilarityFunction
	numCandidatesMul float64
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: create embedding client: %w", err)
	}

	store := &Store{
		client:           config.Client,
		indexName:        config.IndexName,
		embeddingField:   config.EmbeddingField,
		contentField:     config.ContentField,
		metadataField:    config.MetadataField,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		dimensions:       config.Dimensions,
		similarity:       config.Similarity,
		numCandidatesMul: config.NumCandidatesMultiplier,
	}

	if err = store.initialize(ctx, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("elasticsearch: initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the index when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	exists, err := s.indexExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if !initSchema {
		return errors.New("elasticsearch: index not found and InitializeSchema is false")
	}

	if s.dimensions <= 0 {
		dimensions, err := s.embeddingClient.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("elasticsearch: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("elasticsearch: Dimensions must be > 0")
	}

	return s.createIndex(ctx)
}

func (s *Store) indexExists(ctx context.Context) (bool, error) {
	resp, err := s.client.Indices.Exists(
		[]string{s.indexName},
		s.client.Indices.Exists.WithContext(ctx),
	)
	if err != nil {
		return false, fmt.Errorf("indices.exists %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		return true, nil
	case 404:
		return false, nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("indices.exists %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(body))
	}
}

func (s *Store) createIndex(ctx context.Context) error {
	properties := map[string]any{
		s.contentField: map[string]any{"type": "text"},
		s.embeddingField: map[string]any{
			"type":       "dense_vector",
			"dims":       s.dimensions,
			"similarity": string(s.similarity),
			"index":      true,
		},
	}
	if s.metadataField != "" {
		properties[s.metadataField] = map[string]any{
			"type":    "object",
			"dynamic": true,
		}
	}
	body := map[string]any{
		"mappings": map[string]any{"properties": properties},
	}
	buf, err := jsonReader(body)
	if err != nil {
		return err
	}

	resp, err := s.client.Indices.Create(
		s.indexName,
		s.client.Indices.Create.WithBody(buf),
		s.client.Indices.Create.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("indices.create %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("indices.create %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(body))
	}
	return nil
}

// Index embeds the documents and bulk-indexes them.
