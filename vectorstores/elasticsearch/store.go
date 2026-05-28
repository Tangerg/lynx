package elasticsearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdmath "math"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
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
	DocumentBatcher document.Batcher

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

func (c StoreConfig) Validate() error {
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

var _ vectorstore.Store = (*Store)(nil)

// Store is an Elasticsearch-backed implementation of
// [vectorstore.Store]. It uses the dense_vector field type and the
// `knn` query for similarity search.
type Store struct {
	client           *elasticsearch.Client
	indexName        string
	embeddingField   string
	contentField     string
	metadataField    string
	embeddingModel   embedding.Model
	embeddingClient  *embedding.Client
	documentBatcher  document.Batcher
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
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
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

// Create embeds the documents and bulk-indexes them.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("elasticsearch: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "elasticsearch", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("elasticsearch: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("elasticsearch: failed to generate embeddings: %w", err)
		}

		var body bytes.Buffer
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}

			actionLine, err := json.Marshal(map[string]any{
				"index": map[string]any{
					"_index": s.indexName,
					"_id":    id,
				},
			})
			if err != nil {
				return fmt.Errorf("elasticsearch: encode bulk action: %w", err)
			}

			docBody := map[string]any{
				s.contentField:   doc.Text,
				s.embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			}
			if s.metadataField != "" {
				docBody[s.metadataField] = doc.Metadata
			} else {
				for k, v := range doc.Metadata {
					docBody[k] = v
				}
			}
			docLine, err := json.Marshal(docBody)
			if err != nil {
				return fmt.Errorf("elasticsearch: encode bulk doc: %w", err)
			}

			body.Write(actionLine)
			body.WriteByte('\n')
			body.Write(docLine)
			body.WriteByte('\n')
		}

		resp, err := s.client.Bulk(
			bytes.NewReader(body.Bytes()),
			s.client.Bulk.WithContext(ctx),
		)
		if err != nil {
			return fmt.Errorf("elasticsearch: bulk: %w", err)
		}
		if err = parseBulkResponse(resp); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve runs a KNN search over the embedding field. Optional
// metadata filtering is expressed via a query_string clause.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("elasticsearch: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "elasticsearch", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	knn := map[string]any{
		"field":          s.embeddingField,
		"query_vector":   queryVec,
		"k":              req.TopK,
		"num_candidates": int(stdmath.Ceil(float64(req.TopK) * s.numCandidatesMul)),
	}

	filterQuery, err := s.buildFilterQuery(req.Filter)
	if err != nil {
		return nil, err
	}
	if filterQuery != "" {
		knn["filter"] = map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		}
	}

	body := map[string]any{
		"size": req.TopK,
		"knn":  knn,
	}
	buf, err := jsonReader(body)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Search(
		s.client.Search.WithContext(ctx),
		s.client.Search.WithIndex(s.indexName),
		s.client.Search.WithBody(buf),
	)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch: search %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(body))
	}

	var parsed searchResponse
	if err = json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("elasticsearch: decode search response: %w", err)
	}

	docs = make([]*document.Document, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		score := s.normalizeScore(hit.Score)
		if score < req.MinScore {
			continue
		}
		docs = append(docs, s.toDocument(hit, score))
	}
	return docs, nil
}

// Delete removes documents matching the filter via delete_by_query.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("elasticsearch: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "elasticsearch")
	defer func() { tracing.Finish(span, err) }()

	var filterQuery string
	filterQuery, err = s.buildFilterQuery(req.Filter)
	if err != nil {
		return err
	}
	if filterQuery == "" {
		return errors.New("elasticsearch: refusing to delete on empty filter")
	}

	body := map[string]any{
		"query": map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		},
	}
	buf, err := jsonReader(body)
	if err != nil {
		return err
	}

	resp, err := s.client.DeleteByQuery(
		[]string{s.indexName},
		buf,
		s.client.DeleteByQuery.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("elasticsearch: delete_by_query %s: %w", s.indexName, err)
	}
	defer resp.Body.Close()
	if resp.IsError() {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("elasticsearch: delete_by_query %s: status=%d body=%s",
			s.indexName, resp.StatusCode, string(respBody))
	}
	return nil
}

// buildFilterQuery converts the AST filter into a Lucene query string
// for `query_string`. Returns "" when filter is nil.
func (s *Store) buildFilterQuery(filter ast.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataField)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", fmt.Errorf("elasticsearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

// normalizeScore reverses Elasticsearch's vector-score transform to
// produce a [0, 1] similarity score, matching Spring AI's mapping.
//
//	cosine / dot_product:  ES already returns (sim+1)/2 in [0, 1];
//	                       map back via (2*score - 1).
//	l2_norm:               ES returns 1/(1+d²); invert via the closed
//	                       form to recover sim ≈ 1 - sqrt(1/score - 1).
func (s *Store) normalizeScore(score float64) float64 {
	var sim float64
	switch s.similarity {
	case SimilarityL2:
		if score <= 0 {
			return 0
		}
		// Recover the underlying distance, then map to [0, 1].
		inner := 1.0/score - 1.0
		if inner < 0 {
			inner = 0
		}
		sim = 1.0 - stdmath.Sqrt(inner)
	case SimilarityDotProduct, SimilarityCosine:
		fallthrough
	default:
		sim = 2.0*score - 1.0
	}
	switch {
	case sim < 0:
		return 0
	case sim > 1:
		return 1
	default:
		return sim
	}
}

func (s *Store) toDocument(hit searchHit, score float64) *document.Document {
	doc := &document.Document{
		ID:    hit.ID,
		Score: score,
	}
	if hit.Source == nil {
		return doc
	}

	// Pull the document text from the configured content field.
	if raw, ok := hit.Source[s.contentField]; ok {
		if s, ok := raw.(string); ok {
			doc.Text = s
		}
	}

	if s.metadataField != "" {
		if rawMeta, ok := hit.Source[s.metadataField]; ok {
			if m, ok := rawMeta.(map[string]any); ok {
				doc.Metadata = m
			}
		}
	} else {
		// Metadata was flattened onto the root — strip the
		// reserved fields and surface the rest.
		meta := make(map[string]any, len(hit.Source))
		for k, v := range hit.Source {
			if k == s.contentField || k == s.embeddingField {
				continue
			}
			meta[k] = v
		}
		if len(meta) > 0 {
			doc.Metadata = meta
		}
	}
	return doc
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.client,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

// searchResponse / searchHit / bulkResponse mirror the slice of the
// Elasticsearch REST response the store actually consumes. We avoid
// the full typed client to keep the dependency footprint small.
type searchResponse struct {
	Hits struct {
		Hits []searchHit `json:"hits"`
	} `json:"hits"`
}

type searchHit struct {
	ID     string         `json:"_id"`
	Score  float64        `json:"_score"`
	Source map[string]any `json:"_source"`
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Index *struct {
			ID     string         `json:"_id"`
			Status int            `json:"status"`
			Error  map[string]any `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

func parseBulkResponse(resp *esapi.Response) error {
	defer resp.Body.Close()
	if resp.IsError() {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("elasticsearch: bulk: status=%d body=%s",
			resp.StatusCode, string(body))
	}
	var parsed bulkResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("elasticsearch: decode bulk response: %w", err)
	}
	if !parsed.Errors {
		return nil
	}
	var firstErr, failedID string
	for _, item := range parsed.Items {
		if item.Index != nil && item.Index.Error != nil {
			failedID = item.Index.ID
			if reason, ok := item.Index.Error["reason"].(string); ok {
				firstErr = reason
			}
			break
		}
	}
	if firstErr == "" {
		firstErr = "unknown error"
	}
	return fmt.Errorf("elasticsearch: bulk failed on id=%s: %s", failedID, firstErr)
}

// jsonReader marshals v to JSON and returns it as an io.Reader.
func jsonReader(v any) (io.Reader, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: encode request: %w", err)
	}
	return bytes.NewReader(buf), nil
}
