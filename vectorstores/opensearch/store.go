package opensearch

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/Tangerg/scope/core/embeddingclient"
	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/core/vectorstore"
	"github.com/Tangerg/scope/core/vectorstore/filter"
)

const Provider = "OpenSearch"

const (
	DefaultIndexName      = "scope-vector-index"
	DefaultEmbeddingField = "embedding"
	DefaultContentField   = "content"
	DefaultMetadataField  = "metadata"
	DefaultSpaceType      = SpaceTypeCosine
	DefaultEngine         = EngineLucene
	DefaultMethodName     = "hnsw"
	bulkRecordSeparator   = '\n'
	mappingTypeText       = "text"
	mappingTypeVector     = "knn_vector"
	mappingTypeObject     = "object"
)

type bulkOperation string

const (
	bulkOperationIndex  bulkOperation = "index"
	bulkOperationDelete bulkOperation = "delete"
)

type createIndexRequest struct {
	Settings indexSettings `json:"settings"`
	Mappings indexMappings `json:"mappings"`
}

type indexSettings struct {
	KNN bool `json:"index.knn"`
}

type indexMappings struct {
	Properties map[string]any `json:"properties"`
}

type textFieldMapping struct {
	Type string `json:"type"`
}

type vectorFieldMapping struct {
	Type       string           `json:"type"`
	Dimensions int              `json:"dimension"`
	Method     annMethodMapping `json:"method"`
}

type annMethodMapping struct {
	Name      string    `json:"name"`
	Engine    Engine    `json:"engine"`
	SpaceType SpaceType `json:"space_type"`
}

type objectFieldMapping struct {
	Type    string `json:"type"`
	Dynamic bool   `json:"dynamic"`
}

type bulkAction struct {
	Index  *bulkActionTarget `json:"index,omitempty"`
	Delete *bulkActionTarget `json:"delete,omitempty"`
}

type bulkActionTarget struct {
	Index string `json:"_index,omitempty"`
	ID    string `json:"_id"`
}

type queryString struct {
	Query string `json:"query"`
}

type queryClause struct {
	QueryString queryString `json:"query_string"`
}

type nearestNeighbor struct {
	Vector []float32    `json:"vector"`
	K      int          `json:"k"`
	Filter *queryClause `json:"filter,omitempty"`
}

type nearestNeighborQuery struct {
	KNN map[string]nearestNeighbor `json:"knn"`
}

type searchRequest struct {
	Size  int                  `json:"size"`
	Query nearestNeighborQuery `json:"query"`
}

type deleteByQueryRequest struct {
	Query queryClause `json:"query"`
}

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

func (s SpaceType) Valid() bool {
	switch s {
	case SpaceTypeCosine, SpaceTypeL2, SpaceTypeIP, SpaceTypeL1, SpaceTypeLInf:
		return true
	default:
		return false
	}
}

func (s SpaceType) String() string { return string(s) }

func (s SpaceType) score(raw float64) vectorstore.Score {
	if s != SpaceTypeIP {
		// OpenSearch already maps cosine and distance spaces to [0,1].
		return vectorstore.ScoreFromValue(raw)
	}

	// For every supported engine, inner-product scores above 1 encode a
	// positive product as product+1. Scores at or below 1 encode a
	// non-positive product as 1/(1-product). Recover the product before
	// applying Scope's unbounded inner-product normalization.
	var product float64
	switch {
	case raw > 1:
		product = raw - 1
	case raw > 0:
		product = 1 - 1/raw
	default:
		return vectorstore.ScoreFromValue(raw)
	}
	return vectorstore.ScoreFromInnerProduct(product)
}

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

func (e Engine) Valid() bool {
	switch e {
	case EngineLucene, EngineNMSLib, EngineFaiss:
		return true
	default:
		return false
	}
}

func (e Engine) String() string { return string(e) }

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
	DocumentBatcher vectorstore.Batcher

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

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Client == nil {
		return errors.New("opensearch: client is required")
	}
	if s.EmbeddingModel == nil {
		return errors.New("opensearch: embedding model is required")
	}
	if s.DocumentBatcher == nil {
		return errors.New("opensearch: document batcher is required")
	}
	if s.Dimensions < 0 {
		return errors.New("opensearch: dimensions must be non-negative")
	}
	if !s.SpaceType.Valid() {
		return fmt.Errorf("opensearch: unsupported space type %q", s.SpaceType)
	}
	if !s.Engine.Valid() {
		return fmt.Errorf("opensearch: unsupported engine %q", s.Engine)
	}
	switch s.MethodName {
	case "hnsw":
	case "ivf":
		if s.Engine != EngineFaiss {
			return fmt.Errorf("opensearch: method %q requires the Faiss engine", s.MethodName)
		}
	default:
		return fmt.Errorf("opensearch: unsupported method name %q", s.MethodName)
	}
	if s.Engine == EngineLucene && (s.SpaceType == SpaceTypeL1 || s.SpaceType == SpaceTypeLInf) {
		return fmt.Errorf("opensearch: Lucene does not support space type %q", s.SpaceType)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (s *StoreConfig) applyDefaults() {
	s.IndexName = cmp.Or(s.IndexName, DefaultIndexName)
	s.EmbeddingField = cmp.Or(s.EmbeddingField, DefaultEmbeddingField)
	s.ContentField = cmp.Or(s.ContentField, DefaultContentField)
	s.MetadataField = cmp.Or(s.MetadataField, DefaultMetadataField)
	s.SpaceType = cmp.Or(s.SpaceType, DefaultSpaceType)
	s.Engine = cmp.Or(s.Engine, DefaultEngine)
	s.MethodName = cmp.Or(s.MethodName, DefaultMethodName)
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
	embeddingClient embeddingclient.Client
	documentBatcher vectorstore.Batcher
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
		return fmt.Errorf("opensearch: index %q does not exist and schema initialization is disabled", s.indexName)
	}

	if s.dimensions <= 0 {
		dimensions, err := s.embeddingClient.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("opensearch: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("opensearch: embedding dimensions must be positive")
	}

	return s.createIndex(ctx)
}

func (s *Store) indexExists(ctx context.Context) (bool, error) {
	resp, err := s.client.Indices.Exists(ctx, opensearchapi.IndicesExistsReq{Indices: []string{s.indexName}})
	if err != nil {
		return false, fmt.Errorf("opensearch: check index %q: %w", s.indexName, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return false, fmt.Errorf("opensearch: read index existence error for %q with status %d: %w",
				s.indexName, resp.StatusCode, readErr)
		}
		return false, fmt.Errorf("opensearch: check index %q: status=%d body=%s",
			s.indexName, resp.StatusCode, string(body))
	}
}

func (s *Store) createIndex(ctx context.Context) error {
	embeddingMapping := vectorFieldMapping{
		Type:       mappingTypeVector,
		Dimensions: s.dimensions,
		Method: annMethodMapping{
			Name: s.methodName, Engine: s.engine, SpaceType: s.spaceType,
		},
	}
	properties := map[string]any{
		s.contentField:   textFieldMapping{Type: mappingTypeText},
		s.embeddingField: embeddingMapping,
	}
	if s.metadataField != "" {
		properties[s.metadataField] = objectFieldMapping{Type: mappingTypeObject, Dynamic: true}
	}

	body, err := encodeJSONRequest(createIndexRequest{
		Settings: indexSettings{KNN: true},
		Mappings: indexMappings{Properties: properties},
	})
	if err != nil {
		return err
	}

	resp, err := s.client.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: s.indexName,
		Body:  body,
	})
	if err != nil {
		return fmt.Errorf("opensearch: create index %q: %w", s.indexName, err)
	}
	if resp != nil && resp.Inspect().Response != nil && resp.Inspect().Response.IsError() {
		response := resp.Inspect().Response
		raw, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return fmt.Errorf("opensearch: read create-index error for %q with status %d: %w",
				s.indexName, response.StatusCode, readErr)
		}
		return fmt.Errorf("opensearch: create index %q: status=%d body=%s",
			s.indexName, response.StatusCode, string(raw))
	}
	return nil
}

func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return fmt.Errorf("opensearch.Store.Index: %w", validateErr)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("opensearch: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("opensearch: embed documents: %w", err)
		}

		var body bytes.Buffer
		for index, doc := range docs {
			id := doc.ID

			actionLine, encErr := json.Marshal(bulkAction{
				Index: &bulkActionTarget{ID: id},
			})
			if encErr != nil {
				return fmt.Errorf("opensearch: encode bulk action: %w", encErr)
			}

			docBody := map[string]any{
				s.contentField:   doc.Text,
				s.embeddingField: embedding.Float32Vector(vectors[index]),
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
			body.WriteByte(bulkRecordSeparator)
			body.Write(docLine)
			body.WriteByte(bulkRecordSeparator)
		}

		resp, err := s.client.Bulk(ctx, opensearchapi.BulkReq{
			Index: s.indexName,
			Body:  bytes.NewReader(body.Bytes()),
		})
		if err != nil {
			return fmt.Errorf("opensearch: bulk: %w", err)
		}
		if err := (bulkOutcome{operation: bulkOperationIndex, response: resp}).Err(); err != nil {
			return err
		}
	}
	return nil
}

type bulkOutcome struct {
	operation bulkOperation
	response  *opensearchapi.BulkResp
}

func (b bulkOutcome) Err() error {
	if b.response == nil {
		return fmt.Errorf("opensearch: bulk %s returned no response", b.operation)
	}
	if !b.response.Errors {
		return nil
	}
	for _, item := range b.response.Items {
		for _, info := range item {
			if info.Error != nil {
				reason := info.Error.Reason
				if reason == "" {
					reason = "provider returned no reason"
				}
				return fmt.Errorf("opensearch: bulk %s failed for document %q with status %d: %s",
					b.operation, info.ID, info.Status, reason)
			}
		}
	}
	return fmt.Errorf("opensearch: bulk %s reported errors without an item failure", b.operation)
}

func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("opensearch.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("opensearch: embed query: %w", err)
	}
	queryVec := embedding.Float32Vector(vector)

	neighbor := nearestNeighbor{
		Vector: queryVec,
		K:      req.Options.TopK,
	}
	filterQuery, err := s.buildFilterQuery(req.Options.Filter)
	if err != nil {
		return nil, err
	}
	if filterQuery != "" {
		neighbor.Filter = &queryClause{QueryString: queryString{Query: filterQuery}}
	}

	body, err := encodeJSONRequest(searchRequest{
		Size: req.Options.TopK,
		Query: nearestNeighborQuery{
			KNN: map[string]nearestNeighbor{s.embeddingField: neighbor},
		},
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

	docs = make([]*vectorstore.SearchResult, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		score := s.spaceType.score(float64(hit.Score))
		if score < req.Options.MinScore {
			continue
		}
		doc, err := s.toDocument(hit)
		if err != nil {
			return nil, err
		}
		docs = append(docs, &vectorstore.SearchResult{Document: doc, Score: score})
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
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

	body, err := encodeJSONRequest(deleteByQueryRequest{
		Query: queryClause{QueryString: queryString{Query: filterQuery}},
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

func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	var body bytes.Buffer
	for _, id := range ids {
		var actionLine []byte
		actionLine, err = json.Marshal(bulkAction{
			Delete: &bulkActionTarget{Index: s.indexName, ID: id},
		})
		if err != nil {
			return fmt.Errorf("opensearch: encode bulk delete action: %w", err)
		}
		body.Write(actionLine)
		body.WriteByte(bulkRecordSeparator)
	}

	resp, err := s.client.Bulk(ctx, opensearchapi.BulkReq{
		Index: s.indexName,
		Body:  bytes.NewReader(body.Bytes()),
	})
	if err != nil {
		return fmt.Errorf("opensearch: bulk delete: %w", err)
	}
	return (bulkOutcome{operation: bulkOperationDelete, response: resp}).Err()
}

// buildFilterQuery wraps the visitor and returns the Lucene query
// string suitable for the knn filter.
func (s *Store) buildFilterQuery(filter filter.Predicate) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(s.metadataField)
	if err := filter.Accept(v); err != nil {
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

func encodeJSONRequest(value any) (io.Reader, error) {
	buf, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("opensearch: encode request: %w", err)
	}
	return bytes.NewReader(buf), nil
}
