package opensearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "OpenSearch"

const (
	DefaultIndexName      = "lynx-vector-index"
	DefaultEmbeddingField = "embedding"
	DefaultContentField   = "content"
	DefaultMetadataField  = "metadata"
	DefaultDimensions     = 1536
	DefaultSpaceType      = SpaceTypeCosine
	DefaultEngine         = EngineLucene
	DefaultMethodName     = "hnsw"
)

// SpaceType selects the vector similarity space recognized by
// OpenSearch's knn_vector field. The chosen value is baked into the
// index mapping; changing it after the index is created has no effect.
type SpaceType string

const (
	// SpaceTypeCosine — cosine similarity ("cosinesimil"). Default.
	SpaceTypeCosine SpaceType = "cosinesimil"

	// SpaceTypeL2 — squared L2 distance.
	SpaceTypeL2 SpaceType = "l2"

	// SpaceTypeIP — inner product. Only supported by the
	// nmslib / faiss engines.
	SpaceTypeIP SpaceType = "innerproduct"

	// SpaceTypeL1 — Manhattan distance. nmslib / faiss only.
	SpaceTypeL1 SpaceType = "l1"

	// SpaceTypeLInf — Chebyshev (L∞) distance. nmslib / faiss only.
	SpaceTypeLInf SpaceType = "linf"
)

// Engine selects the underlying ANN library that backs the knn_vector
// field. Lucene is the default — it ships with every recent OpenSearch
// release and supports cosine / l2 / innerproduct. The nmslib and
// faiss engines unlock l1 / linf and other advanced parameters but
// must be installed as plugins.
type Engine string

const (
	// EngineLucene — Apache Lucene HNSW. Default; ships with
	// OpenSearch core.
	EngineLucene Engine = "lucene"

	// EngineNMSLib — Non-Metric Space Library.
	EngineNMSLib Engine = "nmslib"

	// EngineFaiss — Meta's FAISS library.
	EngineFaiss Engine = "faiss"
)

// StoreConfig contains configuration options for the OpenSearch vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// Client is the opensearchapi typed client. Required.
	Client *opensearchapi.Client

	// IndexName names the OpenSearch index. Optional: defaults to
	// [DefaultIndexName].
	IndexName string

	// EmbeddingField is the knn_vector field name. Optional:
	// defaults to [DefaultEmbeddingField].
	EmbeddingField string

	// ContentField stores the document text. Optional: defaults to
	// [DefaultContentField].
	ContentField string

	// MetadataField is the object field that holds metadata.
	// Optional: defaults to [DefaultMetadataField]. Pass "" to flatten
	// metadata onto the document root (filters then reference bare
	// field names).
	MetadataField string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before bulk upsert. Required.
	DocumentBatcher document.Batcher

	// Dimensions registered with the knn_vector field. When zero the
	// store asks the embedding model and falls back to
	// [DefaultDimensions].
	Dimensions int

	// SpaceType selects the similarity space. Optional: defaults to
	// [SpaceTypeCosine].
	SpaceType SpaceType

	// Engine selects the ANN engine. Optional: defaults to
	// [EngineLucene].
	Engine Engine

	// MethodName is the ANN method recorded in the field mapping;
	// `hnsw` is the only option supported by the Lucene engine. Set
	// to "ivf" together with [EngineFaiss] to use IVF.
	// Optional: defaults to "hnsw".
	MethodName string

	// InitializeSchema, when true, creates the index with the right
	// mapping when missing. When false and the index doesn't exist,
	// [NewStore] returns [ErrIndexMissing].
	InitializeSchema bool
}

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("opensearch: Client is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("opensearch: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("opensearch: DocumentBatcher is required")
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
	c.MetadataField = cmp.Or(c.MetadataField, DefaultMetadataField)
	c.SpaceType = cmp.Or(c.SpaceType, DefaultSpaceType)
	c.Engine = cmp.Or(c.Engine, DefaultEngine)
	c.MethodName = cmp.Or(c.MethodName, DefaultMethodName)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is an OpenSearch-backed [vectorstore.Store] implementation.
type Store struct {
	client          *opensearchapi.Client
	indexName       string
	embeddingField  string
	contentField    string
	metadataField   string
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher document.Batcher
	dimensions      int
	spaceType       SpaceType
	engine          Engine
	methodName      string
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("opensearch: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:          config.Client,
		indexName:       config.IndexName,
		embeddingField:  config.EmbeddingField,
		contentField:    config.ContentField,
		metadataField:   config.MetadataField,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
		spaceType:       config.SpaceType,
		engine:          config.Engine,
		methodName:      config.MethodName,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("opensearch: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the index when needed.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("opensearch: Dimensions must be > 0")
	}

	exists, err := s.indexExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if !initSchema {
		return errors.New("opensearch: index not found and InitializeSchema is false")
	}
	return s.createIndex(ctx)
}

func (s *Store) indexExists(ctx context.Context) (bool, error) {
	resp, err := s.client.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{Indices: []string{s.indexName}})
	if err != nil {
		return false, fmt.Errorf("indices.exists: %w", err)
	}
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
	embeddingMapping := map[string]any{
		"type":      "knn_vector",
		"dimension": s.dimensions,
		"method": map[string]any{
			"name":       s.methodName,
			"engine":     string(s.engine),
			"space_type": string(s.spaceType),
		},
	}
	properties := map[string]any{
		s.contentField:   map[string]any{"type": "text"},
		s.embeddingField: embeddingMapping,
	}
	if s.metadataField != "" {
		properties[s.metadataField] = map[string]any{
			"type":    "object",
			"dynamic": true,
		}
	}

	body, err := jsonReader(map[string]any{
		"settings": map[string]any{"index.knn": true},
		"mappings": map[string]any{"properties": properties},
	})
	if err != nil {
		return err
	}

	resp, err := s.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: s.indexName,
		Body:  body,
	})
	if err != nil {
		return fmt.Errorf("indices.create %s: %w", s.indexName, err)
	}
	if resp != nil && resp.Inspect().Response != nil && resp.Inspect().Response.IsError() {
		raw, _ := io.ReadAll(resp.Inspect().Response.Body)
		return fmt.Errorf("indices.create %s: status=%d body=%s",
			s.indexName, resp.Inspect().Response.StatusCode, string(raw))
	}
	return nil
}

// Create embeds documents and bulk-indexes them.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("opensearch: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "opensearch", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("opensearch: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("opensearch: failed to generate embeddings: %w", err)
		}

		var body bytes.Buffer
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}

			actionLine, err := json.Marshal(map[string]any{
				"index": map[string]any{"_id": id},
			})
			if err != nil {
				return fmt.Errorf("opensearch: encode bulk action: %w", err)
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
				return fmt.Errorf("opensearch: encode bulk doc: %w", err)
			}

			body.Write(actionLine)
			body.WriteByte('\n')
			body.Write(docLine)
			body.WriteByte('\n')
		}

		resp, err := s.client.Bulk(ctx, opensearchapi.BulkReq{
			Index: s.indexName,
			Body:  bytes.NewReader(body.Bytes()),
		})
		if err != nil {
			return fmt.Errorf("opensearch: bulk: %w", err)
		}
		if resp != nil && resp.Errors {
			return s.bulkErrorReason(resp)
		}
	}
	return nil
}

func (s *Store) bulkErrorReason(resp *opensearchapi.BulkResp) error {
	for _, item := range resp.Items {
		for _, info := range item {
			if info.Error != nil {
				return fmt.Errorf("opensearch: bulk failed on id=%s: %s",
					info.ID, info.Error.Reason)
			}
		}
	}
	return errors.New("opensearch: bulk reported errors with no item-level reason")
}

// Retrieve runs an approximate KNN query against the configured index
// and returns the documents above MinScore.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("opensearch: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "opensearch", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("opensearch: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	knnQuery := map[string]any{
		s.embeddingField: map[string]any{
			"vector": queryVec,
			"k":      req.TopK,
		},
	}
	filterQuery, err := s.buildFilterQuery(req.Filter)
	if err != nil {
		return nil, err
	}
	if filterQuery != "" {
		knnQuery[s.embeddingField].(map[string]any)["filter"] = map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		}
	}

	body, err := jsonReader(map[string]any{
		"size":  req.TopK,
		"query": map[string]any{"knn": knnQuery},
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{s.indexName},
		Body:    body,
	})
	if err != nil {
		return nil, fmt.Errorf("opensearch: search %s: %w", s.indexName, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("opensearch: nil response for %s", s.indexName)
	}

	docs = make([]*document.Document, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		score := float64(hit.Score)
		if score < req.MinScore {
			continue
		}
		doc, err := s.toDocument(hit, score)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// Delete removes documents matching the filter expression via
// delete_by_query.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("opensearch: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "opensearch")
	defer func() { tracing.Finish(span, err) }()

	var filterQuery string
	filterQuery, err = s.buildFilterQuery(req.Filter)
	if err != nil {
		return err
	}
	if filterQuery == "" {
		return errors.New("opensearch: refusing to delete on empty filter")
	}

	body, err := jsonReader(map[string]any{
		"query": map[string]any{
			"query_string": map[string]any{"query": filterQuery},
		},
	})
	if err != nil {
		return err
	}

	resp, err := s.client.Document.DeleteByQuery(ctx, opensearchapi.DocumentDeleteByQueryReq{
		Indices: []string{s.indexName},
		Body:    body,
	})
	if err != nil {
		return fmt.Errorf("opensearch: delete_by_query %s: %w", s.indexName, err)
	}
	if resp != nil && len(resp.Failures) > 0 {
		return fmt.Errorf("opensearch: delete_by_query %s reported %d failures",
			s.indexName, len(resp.Failures))
	}
	return nil
}

// buildFilterQuery wraps the visitor and returns the Lucene query
// string suitable for the knn filter.
func (s *Store) buildFilterQuery(filter ast.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataField)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", fmt.Errorf("opensearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(hit opensearchapi.SearchHit, score float64) (*document.Document, error) {
	doc := &document.Document{ID: hit.ID, Score: score}
	if len(hit.Source) == 0 {
		return doc, nil
	}

	var source map[string]any
	if err := json.Unmarshal(hit.Source, &source); err != nil {
		return nil, fmt.Errorf("opensearch: decode _source for %s: %w", hit.ID, err)
	}

	if raw, ok := source[s.contentField]; ok {
		if str, ok := raw.(string); ok {
			doc.Text = str
		}
	}

	if s.metadataField != "" {
		if rawMeta, ok := source[s.metadataField]; ok {
			if m, ok := rawMeta.(map[string]any); ok {
				doc.Metadata = m
			}
		}
	} else {
		meta := make(map[string]any, len(source))
		for k, v := range source {
			switch k {
			case s.contentField, s.embeddingField:
				continue
			}
			meta[k] = v
		}
		if len(meta) > 0 {
			doc.Metadata = meta
		}
	}
	return doc, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.client,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

// jsonReader marshals v to JSON and returns it as an io.Reader.
func jsonReader(v any) (io.Reader, error) {
	buf, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("opensearch: encode request: %w", err)
	}
	return bytes.NewReader(buf), nil
}
