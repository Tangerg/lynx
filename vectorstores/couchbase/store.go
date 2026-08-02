package couchbase

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/couchbase/gocb/v2"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/vectorstores/internal/batching"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/scores"
	vectorconv "github.com/Tangerg/lynx/vectorstores/internal/vector"
)

const Provider = "Couchbase"

const (
	DefaultScopeName      = "_default"
	DefaultCollectionName = "_default"
	DefaultIndexName      = "lynx-vector-index"
	DefaultSimilarity     = SimilarityDotProduct
	DefaultIndexOptimize  = OptimizeRecall
	contentField          = "content"
	embeddingField        = "embedding"
	metadataField         = "metadata"
	idField               = "id"
	resultScoreField      = "_lynx_score"
)

// Similarity selects the vector similarity function written into the
// Couchbase search-index definition.
type Similarity string

const (
	// SimilarityCosine — cosine similarity.
	SimilarityCosine Similarity = "cosine"

	// SimilarityL2Norm — L2 (Euclidean) norm.
	SimilarityL2Norm Similarity = "l2_norm"

	// SimilarityDotProduct — dot product. Default; works
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

// StoreConfig contains configuration options for the Couchbase Search
// vector store.
type StoreConfig struct {
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
	DocumentBatcher vectorstore.Batcher

	// Dimensions sets the vector width registered with the search index. When
	// zero and InitializeSchema is true, the store probes EmbeddingModel.
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

func (c StoreConfig) Validate() error {
	c.applyDefaults()
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
	if c.Dimensions < 0 {
		return errors.New("couchbase: Dimensions must be >= 0")
	}
	switch c.Similarity {
	case SimilarityCosine, SimilarityL2Norm, SimilarityDotProduct:
	default:
		return fmt.Errorf("couchbase: unsupported Similarity %q", c.Similarity)
	}
	switch c.IndexOptimization {
	case OptimizeRecall, OptimizeLatency, OptimizeMemory:
	default:
		return fmt.Errorf("couchbase: unsupported IndexOptimization %q", c.IndexOptimization)
	}
	return ident.CheckWithDash("couchbase", map[string]string{
		"BucketName":      c.BucketName,
		"ScopeName":       c.ScopeName,
		"CollectionName":  c.CollectionName,
		"VectorIndexName": c.VectorIndexName,
	})
}

// applyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) applyDefaults() {
	c.ScopeName = cmp.Or(c.ScopeName, DefaultScopeName)
	c.CollectionName = cmp.Or(c.CollectionName, DefaultCollectionName)
	c.VectorIndexName = cmp.Or(c.VectorIndexName, DefaultIndexName)
	c.Similarity = cmp.Or(c.Similarity, DefaultSimilarity)
	c.IndexOptimization = cmp.Or(c.IndexOptimization, DefaultIndexOptimize)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store implements vector-store capabilities with Couchbase Search Service.
type Store struct {
	cluster           *gocb.Cluster
	bucket            *gocb.Bucket
	scope             *gocb.Scope
	collection        *gocb.Collection
	bucketName        string
	scopeName         string
	collectionName    string
	vectorIndexName   string
	embeddingClient   *embeddingclient.Client
	documentBatcher   vectorstore.Batcher
	dimensions        int
	similarity        Similarity
	indexOptimization IndexOptimization
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("couchbase: create embedding client: %w", err)
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
		embeddingClient:   embeddingClient,
		documentBatcher:   config.DocumentBatcher,
		dimensions:        config.Dimensions,
		similarity:        config.Similarity,
		indexOptimization: config.IndexOptimization,
	}

	if err = store.initialize(ctx, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("couchbase: initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and creates the search index when
// requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if !initSchema {
		return nil
	}
	if s.dimensions <= 0 {
		dimensions, err := s.embeddingClient.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("couchbase: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("couchbase: Dimensions must be > 0")
	}

	return s.upsertSearchIndex()
}

// upsertSearchIndex creates (or refreshes) the FTS index used for
// vector + content search. The index definition mirrors the one
// the framework generates.
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
									"dims":                       s.dimensions,
									"index":                      true,
									"name":                       embeddingField,
									"similarity":                 string(s.similarity),
									"type":                       "vector",
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

// Add embeds documents and upserts them by id.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("couchbase.Store.Add: %w", err)
	}

	var batchedDocs [][]*document.Document
	batchedDocs, err = batching.Batch(ctx, s.documentBatcher, docs)
	if err != nil {
		return fmt.Errorf("couchbase: batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("couchbase: embed documents: %w", err)
		}

		for i, doc := range docs {
			id := doc.ID
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("couchbase: decode metadata for %s: %w", id, err)
			}
			payload := map[string]any{
				idField:        id,
				contentField:   doc.Text,
				metadataField:  metaOrEmpty(metadataValues),
				embeddingField: vectorconv.Float32(vectors[i]),
			}
			if _, err := s.collection.Upsert(id, payload, &gocb.UpsertOptions{Context: ctx}); err != nil {
				return fmt.Errorf("couchbase: upsert %s: %w", id, err)
			}
		}
	}
	return nil
}

// Search runs a SQL++ query that embeds the KNN search clause.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("couchbase.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("couchbase: embed query: %w", err)
	}
	queryVec := vectorconv.Float32(vector)
	vectorJSON, err := json.Marshal(queryVec)
	if err != nil {
		return nil, fmt.Errorf("couchbase: encode query vector: %w", err)
	}

	whereExtra := ""
	if req.Filter != nil {
		predicate, filterErr := s.buildFilter(req.Filter)
		if filterErr != nil {
			return nil, filterErr
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
		`SELECT c.*, SEARCH_SCORE() AS %s FROM `+"`%s`"+`.`+"`%s`"+`.`+"`%s`"+` AS c `+
			`WHERE SEARCH(c, %s, {"index": "%s"})%s ORDER BY SEARCH_SCORE() DESC LIMIT %d`,
		resultScoreField,
		s.bucketName, s.scopeName, s.collectionName,
		knnFragment, indexFullName, whereExtra, req.TopK,
	)

	rows, err := s.scope.Query(stmt, &gocb.QueryOptions{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("couchbase: query: %w", err)
	}
	defer rows.Close()

	docs = make([]vectorstore.Match, 0, req.TopK)
	for rows.Next() {
		var raw map[string]any
		if err := rows.Row(&raw); err != nil {
			return nil, fmt.Errorf("couchbase: decode row: %w", err)
		}
		doc, err := s.toDocument(raw)
		if err != nil {
			return nil, err
		}
		rawScore, ok := raw[resultScoreField].(float64)
		if !ok {
			return nil, fmt.Errorf("couchbase: result is missing numeric %s", resultScoreField)
		}
		score := scores.Bounded(rawScore)
		if score < req.MinScore {
			continue
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("couchbase: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes documents matching the filter via DELETE.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("couchbase.Store.DeleteWhere: %w", err)
	}

	predicate, err := s.buildFilter(expr)
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

// DeleteIDs removes documents by their KV key. Add upserts each
// document under its id as the document key (see [Store.Add]), so the
// id is the KV key here too. An empty slice is a no-op; a per-key
// "document not found" error is treated as success so repeated deletes
// stay idempotent. Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	for _, id := range ids {
		if _, removeErr := s.collection.Remove(id, &gocb.RemoveOptions{Context: ctx}); removeErr != nil {
			if errors.Is(removeErr, gocb.ErrDocumentNotFound) {
				continue
			}
			return fmt.Errorf("couchbase: remove %s: %w", id, removeErr)
		}
	}
	return nil
}

// buildFilter wraps the visitor.
func (s *Store) buildFilter(expr filter.Predicate) (string, error) {
	if expr == nil {
		return "", nil
	}
	v := NewVisitor(metadataField)
	if err := v.Visit(expr); err != nil {
		return "", fmt.Errorf("couchbase: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toDocument(raw map[string]any) (*document.Document, error) {
	id, ok := raw[idField].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("couchbase: result is missing string field %q", idField)
	}
	content, ok := raw[contentField].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("couchbase: result is missing string field %q", contentField)
	}
	doc := &document.Document{ID: id, Text: content}
	if meta, ok := raw[metadataField].(map[string]any); ok {
		var err error
		doc.Metadata, err = metadata.FromValues(meta)
		if err != nil {
			return nil, fmt.Errorf("couchbase: convert metadata: %w", err)
		}
	}
	return doc, nil
}

// metaOrEmpty returns an empty map when m is nil so the resulting JSON
// document always carries a `metadata` field — easier to deserialize.
func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (s *Store) Close() error { return nil }
