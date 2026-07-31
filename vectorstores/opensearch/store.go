package opensearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/batching"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/scores"
)

const Provider = "OpenSearch"

const (
	DefaultIndexName      = "lynx-vector-index"
	DefaultEmbeddingField = "embedding"
	DefaultContentField   = "content"
	DefaultMetadataField  = "metadata"
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
	DocumentBatcher vectorstores.Batcher

	// Dimensions sets the knn_vector width for a newly created index. When zero,
	// the store probes EmbeddingModel only if it must create the index.
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

func (c StoreConfig) Validate() error {
	c.applyDefaults()
	if c.Client == nil {
		return errors.New("opensearch: Client is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("opensearch: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("opensearch: DocumentBatcher is required")
	}
	if c.Dimensions < 0 {
		return errors.New("opensearch: Dimensions must be >= 0")
	}
	switch c.SpaceType {
	case SpaceTypeCosine, SpaceTypeL2, SpaceTypeIP, SpaceTypeL1, SpaceTypeLInf:
	default:
		return fmt.Errorf("opensearch: unsupported SpaceType %q", c.SpaceType)
	}
	switch c.Engine {
	case EngineLucene, EngineNMSLib, EngineFaiss:
	default:
		return fmt.Errorf("opensearch: unsupported Engine %q", c.Engine)
	}
	switch c.MethodName {
	case "hnsw":
	case "ivf":
		if c.Engine != EngineFaiss {
			return fmt.Errorf("opensearch: method %q requires the Faiss engine", c.MethodName)
		}
	default:
		return fmt.Errorf("opensearch: unsupported MethodName %q", c.MethodName)
	}
	if c.Engine == EngineLucene && (c.SpaceType == SpaceTypeL1 || c.SpaceType == SpaceTypeLInf) {
		return fmt.Errorf("opensearch: Lucene does not support SpaceType %q", c.SpaceType)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) applyDefaults() {
	c.IndexName = cmp.Or(c.IndexName, DefaultIndexName)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.MetadataField = cmp.Or(c.MetadataField, DefaultMetadataField)
	c.SpaceType = cmp.Or(c.SpaceType, DefaultSpaceType)
	c.Engine = cmp.Or(c.Engine, DefaultEngine)
	c.MethodName = cmp.Or(c.MethodName, DefaultMethodName)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store implements vector-store capabilities with OpenSearch.
type Store struct {
	client          *opensearchapi.Client
	indexName       string
	embeddingField  string
	contentField    string
	metadataField   string
	embeddingClient *embeddingclient.Client
	documentBatcher vectorstores.Batcher
	dimensions      int
	spaceType       SpaceType
	engine          Engine
	methodName      string
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("opensearch: create embedding client: %w", err)
	}

	store := &Store{
		client:          config.Client,
		indexName:       config.IndexName,
		embeddingField:  config.EmbeddingField,
		contentField:    config.ContentField,
		metadataField:   config.MetadataField,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
		spaceType:       config.SpaceType,
		engine:          config.Engine,
		methodName:      config.MethodName,
	}

	if err = store.initialize(ctx, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("opensearch: initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the index when needed.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
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

	if s.dimensions <= 0 {
		dimensions, err := s.embeddingClient.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("opensearch: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("opensearch: Dimensions must be > 0")
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

// Add embeds documents and bulk-indexes them.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("opensearch.Store.Add: %w", err)
	}

	var batchedDocs [][]*document.Document
	batchedDocs, err = batching.Batch(ctx, s.documentBatcher, docs)
	if err != nil {
		return fmt.Errorf("opensearch: batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("opensearch: embed documents: %w", err)
		}

		var body bytes.Buffer
		for i, doc := range docs {
			id := doc.ID

			actionLine, encErr := json.Marshal(map[string]any{
				"index": map[string]any{"_id": id},
			})
			if encErr != nil {
				return fmt.Errorf("opensearch: encode bulk action: %w", encErr)
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
			docLine, encErr := json.Marshal(docBody)
			if encErr != nil {
				return fmt.Errorf("opensearch: encode bulk doc: %w", encErr)
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

// Search runs an approximate KNN query against the configured index
// and returns the documents above MinScore.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("opensearch.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("opensearch: embed query: %w", err)
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

	docs = make([]vectorstore.Match, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		score := s.normalizeScore(float64(hit.Score))
		if score < req.MinScore {
			continue
		}
		doc, err := s.toDocument(hit)
		if err != nil {
			return nil, err
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	return docs, nil
}

func (s *Store) normalizeScore(raw float64) float64 {
	if s.spaceType != SpaceTypeIP {
		// OpenSearch already maps cosine and distance spaces to [0,1].
		return scores.Bounded(raw)
	}

	// For every supported engine, inner-product scores above 1 encode a
	// positive product as product+1. Scores at or below 1 encode a
	// non-positive product as 1/(1-product). Recover the product before
	// applying Lynx's unbounded inner-product normalization.
	var product float64
	if raw > 1 {
		product = raw - 1
	} else if raw > 0 {
		product = 1 - 1/raw
	} else {
		return scores.Bounded(raw)
	}
	return scores.InnerProduct(product)
}

// Delete removes documents matching the filter expression via
// delete_by_query.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("opensearch.Store.DeleteWhere: %w", err)
	}

	var filterQuery string
	filterQuery, err = s.buildFilterQuery(expr)
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

// DeleteIDs removes documents by their OpenSearch _id via a single
// Bulk request carrying one delete action per id (NDJSON
// `{"delete":{"_index":idx,"_id":id}}`). An empty slice is a no-op;
// unknown ids are silently ignored (Bulk reports them as not_found, not
// an error). Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	var body bytes.Buffer
	for _, id := range ids {
		var actionLine []byte
		actionLine, err = json.Marshal(map[string]any{
			"delete": map[string]any{"_index": s.indexName, "_id": id},
		})
		if err != nil {
			return fmt.Errorf("opensearch: encode bulk delete action: %w", err)
		}
		body.Write(actionLine)
		body.WriteByte('\n')
	}

	resp, err := s.client.Bulk(ctx, opensearchapi.BulkReq{
		Index: s.indexName,
		Body:  bytes.NewReader(body.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("opensearch: bulk delete: %w", err)
	}
	if resp != nil && resp.Errors {
		return s.bulkErrorReason(resp)
	}
	return nil
}

// buildFilterQuery wraps the visitor and returns the Lucene query
// string suitable for the knn filter.
func (s *Store) buildFilterQuery(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataField)
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("opensearch: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(hit opensearchapi.SearchHit) (*document.Document, error) {
	if hit.ID == "" {
		return nil, errors.New("opensearch: search hit is missing _id")
	}
	doc := &document.Document{ID: hit.ID}
	if len(hit.Source) == 0 {
		return nil, fmt.Errorf("opensearch: search hit %s is missing _source", hit.ID)
	}

	var source map[string]any
	if err := json.Unmarshal(hit.Source, &source); err != nil {
		return nil, fmt.Errorf("opensearch: decode _source for %s: %w", hit.ID, err)
	}

	content, ok := source[s.contentField].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("opensearch: search hit %s is missing string field %q", hit.ID, s.contentField)
	}
	doc.Text = content

	if s.metadataField != "" {
		if rawMeta, ok := source[s.metadataField]; ok {
			if m, ok := rawMeta.(map[string]any); ok {
				var err error
				doc.Metadata, err = metadata.FromValues(m)
				if err != nil {
					return nil, fmt.Errorf("opensearch: convert metadata: %w", err)
				}
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
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("opensearch: convert metadata: %w", err)
			}
		}
	}
	return doc, nil
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
