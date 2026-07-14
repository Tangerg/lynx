package elasticsearch

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/vectorstores"
)

const Provider = "Elasticsearch"

const (
	DefaultIndexName        = "lynx-vector-index"
	DefaultEmbeddingField   = "embedding"
	DefaultContentField     = "content"
	DefaultMetadataField    = "metadata"
	DefaultDimensions       = 1536
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

// StoreConfig contains configuration options for the Elasticsearch
// vector store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

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
	DocumentBatcher vectorstores.Batcher

	// Dimensions sets the dense_vector dims registered with the
	// index. When zero the store asks the embedding model and falls
	// back to [DefaultDimensions].
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

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("elasticsearch: Client is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("elasticsearch: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("elasticsearch: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.IndexName = cmp.Or(c.IndexName, DefaultIndexName)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	if c.MetadataField == "" {
		c.MetadataField = DefaultMetadataField
	}
	c.Similarity = cmp.Or(c.Similarity, DefaultSimilarity)
	if c.NumCandidatesMultiplier <= 0 {
		c.NumCandidatesMultiplier = defaultNumCandidatesMul
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
	embeddingModel   embedding.Model
	embeddingClient  *embedding.Client
	documentBatcher  vectorstores.Batcher
	dimensions       int
	similarity       SimilarityFunction
	numCandidatesMul float64
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:           config.Client,
		indexName:        config.IndexName,
		embeddingField:   config.EmbeddingField,
		contentField:     config.ContentField,
		metadataField:    config.MetadataField,
		embeddingModel:   config.EmbeddingModel,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		dimensions:       config.Dimensions,
		similarity:       config.Similarity,
		numCandidatesMul: config.NumCandidatesMultiplier,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the index when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		dimensions, err := embedding.ResolveDimensions(ctx, s.embeddingModel)
		if err != nil {
			return fmt.Errorf("elasticsearch: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("elasticsearch: Dimensions must be > 0")
	}

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

// Add embeds the documents and bulk-indexes them.
