package couchbase

import (
	"context"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/couchbase/gocb/v2"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "Couchbase"

const (
	DefaultScopeName       = "_default"
	DefaultCollectionName  = "_default"
	DefaultIndexName       = "lynx-vector-index"
	DefaultDimensions      = 1536
	DefaultSimilarity      = SimilarityDotProduct
	DefaultIndexOptimize   = OptimizeRecall
	contentField           = "content"
	embeddingField         = "embedding"
	metadataField          = "metadata"
	idField                = "id"
)

// Similarity selects the vector similarity function written into the
// Couchbase search-index definition.
type Similarity string

const (
	// SimilarityCosine — cosine similarity.
	SimilarityCosine Similarity = "cosine"

	// SimilarityL2Norm — L2 (Euclidean) norm.
	SimilarityL2Norm Similarity = "l2_norm"

	// SimilarityDotProduct — dot product. Default in Spring AI; works
	// best with already-normalized embeddings (e.g. OpenAI).
	SimilarityDotProduct Similarity = "dot_product"
)

// IndexOptimization picks the tradeoff for Couchbase's vector index:
// recall (default), latency, or memory.
type IndexOptimization string

const (
	OptimizeRecall  IndexOptimization = "recall"
	OptimizeLatency IndexOptimization = "latency"
	OptimizeMemory  IndexOptimization = "memory"
)

// safeIdentifier matches Couchbase's allowed identifier set —
// underscores and hyphens are common in bucket / scope / collection /
// index names.

// StoreConfig contains configuration options for the Couchbase Search
// vector store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// Cluster is the connected gocb cluster. Required.
	Cluster *gocb.Cluster

	// BucketName is the Couchbase bucket. Required.
	BucketName string

	// ScopeName is the scope within the bucket. Optional: defaults
	// to [DefaultScopeName] ("_default").
	ScopeName string

	// CollectionName is the collection within the scope. Optional:
	// defaults to [DefaultCollectionName] ("_default").
	CollectionName string

	// VectorIndexName is the search-index name. Optional: defaults
	// to [DefaultIndexName].
	VectorIndexName string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher document.Batcher

	// Dimensions sets the vector width registered with the search
	// index. When zero, falls back to the embedding model's
	// reported value and then [DefaultDimensions].
	Dimensions int

	// Similarity selects the vector similarity function. Optional:
	// defaults to [SimilarityDotProduct].
	Similarity Similarity

	// IndexOptimization selects recall / latency / memory tradeoff.
	// Optional: defaults to [OptimizeRecall].
	IndexOptimization IndexOptimization

	// InitializeSchema, when true, creates the search index if it
	// doesn't already exist.
	InitializeSchema bool
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("couchbase: config must not be nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Cluster == nil {
		return errors.New("couchbase: Cluster is required")
	}
	if c.BucketName == "" {
		return errors.New("couchbase: BucketName is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("couchbase: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("couchbase: DocumentBatcher is required")
	}

	c.ScopeName = cmp.Or(c.ScopeName, DefaultScopeName)
	c.CollectionName = cmp.Or(c.CollectionName, DefaultCollectionName)
	c.VectorIndexName = cmp.Or(c.VectorIndexName, DefaultIndexName)
	c.Similarity = cmp.Or(c.Similarity, DefaultSimilarity)
	c.IndexOptimization = cmp.Or(c.IndexOptimization, DefaultIndexOptimize)

	return ident.CheckWithDash("couchbase", map[string]string{
		"BucketName":      c.BucketName,
		"ScopeName":       c.ScopeName,
		"CollectionName":  c.CollectionName,
		"VectorIndexName": c.VectorIndexName,
	})
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Couchbase Search Service backed [vectorstore.Store].
type Store struct {
	cluster           *gocb.Cluster
	bucket            *gocb.Bucket
	scope             *gocb.Scope
	collection        *gocb.Collection
	bucketName        string
	scopeName         string
	collectionName    string
	vectorIndexName   string
	embeddingModel    embedding.Model
	embeddingClient   *embedding.Client
	documentBatcher   document.Batcher
	dimensions        int
	similarity        Similarity
	indexOptimization IndexOptimization
}


func NewStore(config *StoreConfig) (*Store, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("couchbase: failed to create embedding client: %w", err)
	}

	bucket := config.Cluster.Bucket(config.BucketName)
	scope := bucket.Scope(config.ScopeName)
	collection := scope.Collection(config.CollectionName)

	store := &Store{
		cluster:           config.Cluster,
		bucket:            bucket,
		scope:             scope,
		collection:        collection,
		bucketName:        config.BucketName,
		scopeName:         config.ScopeName,
		collectionName:    config.CollectionName,
		vectorIndexName:   config.VectorIndexName,
		embeddingModel:    config.EmbeddingModel,
		embeddingClient:   embeddingClient,
		documentBatcher:   config.DocumentBatcher,
		dimensions:        config.Dimensions,
		similarity:        config.Similarity,
		indexOptimization: config.IndexOptimization,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("couchbase: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the search index when
// requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("couchbase: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}
	return s.upsertSearchIndex()
}

// upsertSearchIndex creates (or refreshes) the FTS index used for
// vector + content search. The index definition mirrors the one
// Spring AI generates.
func (s *Store) upsertSearchIndex() error {
	mgr := s.scope.SearchIndexes()
	if existing, err := mgr.GetIndex(s.vectorIndexName, nil); err == nil && existing != nil {
		return nil
	}

	typeKey := s.scopeName + "." + s.collectionName
	params := map[string]any{
		"doc_config": map[string]any{
			"docid_prefix_delim": "",
			"docid_regexp":       "",
			"mode":               "scope.collection.type_field",
			"type_field":         "type",
		},
		"mapping": map[string]any{
			"default_analyzer":        "standard",
			"default_datetime_parser": "dateTimeOptional",
			"default_field":           "_all",
			"default_mapping": map[string]any{
				"dynamic": false,
				"enabled": false,
			},
			"default_type":      typeKey,
			"docvalues_dynamic": false,
			"index_dynamic":     false,
			"store_dynamic":     false,
			"type_field":        "_type",
			"types": map[string]any{
				typeKey: map[string]any{
					"dynamic": false,
					"enabled": true,
					"properties": map[string]any{
						embeddingField: map[string]any{
							"dynamic": false,
							"enabled": true,
							"fields": []any{
								map[string]any{
									"dims":                      s.dimensions,
									"index":                     true,
									"name":                      embeddingField,
									"similarity":                string(s.similarity),
									"type":                      "vector",
									"vector_index_optimized_for": string(s.indexOptimization),
								},
							},
						},
						contentField: map[string]any{
							"dynamic": false,
							"enabled": true,
							"fields": []any{
								map[string]any{
									"analyzer":             "keyword",
									"docvalues":            true,
									"include_in_all":       true,
									"include_term_vectors": true,
									"index":                true,
									"name":                 contentField,
									"store":                true,
									"type":                 "text",
								},
							},
						},
					},
				},
			},
		},
		"store": map[string]any{
			"indexType":      "scorch",
			"segmentVersion": 16,
		},
	}

	idx := gocb.SearchIndex{
		Name:       s.vectorIndexName,
		SourceName: s.bucketName,
		Type:       "fulltext-index",
		SourceType: "gocbcore",
		Params:     params,
		PlanParams: map[string]any{
			"maxPartitionsPerPIndex": 1024,
			"indexPartitions":        1,
		},
		SourceParams: map[string]any{},
	}
	return mgr.UpsertIndex(idx, nil)
}

// Create embeds documents and upserts them by id.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("couchbase: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "couchbase", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("couchbase: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("couchbase: failed to generate embeddings: %w", err)
		}

		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			payload := map[string]any{
				idField:        id,
				contentField:   doc.Text,
				metadataField:  metaOrEmpty(doc.Metadata),
				embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			}
			if _, err := s.collection.Upsert(id, payload, &gocb.UpsertOptions{Context: ctx}); err != nil {
				return fmt.Errorf("couchbase: upsert %s: %w", id, err)
			}
		}
	}
	return nil
}

// Retrieve runs a SQL++ query that embeds the KNN search clause.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("couchbase: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "couchbase", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("couchbase: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)
	vectorJSON, err := json.Marshal(queryVec)
	if err != nil {
		return nil, fmt.Errorf("couchbase: encode query vector: %w", err)
	}

	whereExtra := ""
	if req.Filter != nil {
		predicate, err := s.buildFilter(req.Filter)
		if err != nil {
			return nil, err
		}
		if predicate != "" {
			whereExtra = " AND " + predicate
		}
	}

	knnFragment := fmt.Sprintf(
		`{"query":{"match_none":{}},"knn":[{"field":"%s","k":%d,"vector":%s}]}`,
		embeddingField, req.TopK, string(vectorJSON),
	)
	indexFullName := fmt.Sprintf("%s.%s.%s", s.bucketName, s.scopeName, s.vectorIndexName)
	stmt := fmt.Sprintf(
		`SELECT c.* FROM `+"`%s`"+`.`+"`%s`"+`.`+"`%s`"+` AS c `+
			`WHERE SEARCH_SCORE() >= %v AND SEARCH(c, %s, {"index": "%s"})%s LIMIT %d`,
		s.bucketName, s.scopeName, s.collectionName,
		req.MinScore, knnFragment, indexFullName, whereExtra, req.TopK,
	)

	rows, err := s.scope.Query(stmt, &gocb.QueryOptions{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("couchbase: query: %w", err)
	}
	defer rows.Close()

	docs = make([]*document.Document, 0, req.TopK)
	for rows.Next() {
		var raw map[string]any
		if err := rows.Row(&raw); err != nil {
			return nil, fmt.Errorf("couchbase: decode row: %w", err)
		}
		docs = append(docs, s.toDocument(raw))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("couchbase: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes documents matching the filter via DELETE.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("couchbase: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "couchbase")
	defer func() { tracing.Finish(span, err) }()

	predicate, err := s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("couchbase: refusing to delete on empty filter")
	}

	stmt := fmt.Sprintf(
		`DELETE FROM `+"`%s`"+`.`+"`%s`"+`.`+"`%s`"+` WHERE %s`,
		s.bucketName, s.scopeName, s.collectionName, predicate,
	)
	if _, err := s.scope.Query(stmt, &gocb.QueryOptions{Context: ctx}); err != nil {
		return fmt.Errorf("couchbase: delete: %w", err)
	}
	return nil
}

// buildFilter wraps the visitor.
func (s *Store) buildFilter(expr ast.Expr) (string, error) {
	if expr == nil {
		return "", nil
	}
	v := NewVisitor(metadataField)
	v.Visit(expr)
	if err := v.Error(); err != nil {
		return "", fmt.Errorf("couchbase: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(raw map[string]any) *document.Document {
	doc := &document.Document{}
	if id, ok := raw[idField].(string); ok {
		doc.ID = id
	}
	if content, ok := raw[contentField].(string); ok {
		doc.Text = content
	}
	if meta, ok := raw[metadataField].(map[string]any); ok {
		doc.Metadata = meta
	}
	return doc
}

// metaOrEmpty returns an empty map when m is nil so the resulting JSON
// document always carries a `metadata` field — easier to deserialize.
func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (s *Store) Metadata() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: s.cluster,
		Provider:     Provider,
	}
}


func (s *Store) Close() error { return nil }
