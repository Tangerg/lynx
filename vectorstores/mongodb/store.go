package mongodb

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

const Provider = "MongoDB"

const (
	DefaultVectorIndexName = "vector_index"
	DefaultEmbeddingPath   = "embedding"
	DefaultContentField    = "content"
	DefaultMetadataField   = "metadata"
	DefaultNumCandidates   = 200
	defaultIDField         = "_id"
	scoreField             = "score"
)

// Similarity selects the vector similarity function written into the
// Atlas Vector Search index definition.
type Similarity string

const (
	// SimilarityCosine — cosine similarity. Default.
	SimilarityCosine Similarity = "cosine"

	// SimilarityEuclidean — Euclidean (L2) distance.
	SimilarityEuclidean Similarity = "euclidean"

	// SimilarityDotProduct — dot product (best for normalized
	// embeddings).
	SimilarityDotProduct Similarity = "dotProduct"
)

// StoreConfig contains configuration options for the MongoDB Atlas
// Vector Search store.
type StoreConfig struct {
	// Collection is the MongoDB collection that holds the documents.
	// Required.
	Collection *mongo.Collection

	// VectorIndexName is the Atlas Vector Search index name. It must
	// match an existing index (or one created by InitializeSchema).
	// Optional: defaults to [DefaultVectorIndexName].
	VectorIndexName string

	// EmbeddingPath is the field that holds the document embedding.
	// Optional: defaults to [DefaultEmbeddingPath] ("embedding").
	EmbeddingPath string

	// ContentField is the field that stores the original text.
	// Optional: defaults to [DefaultContentField].
	ContentField string

	// MetadataField is the sub-document field that holds metadata.
	// Optional: defaults to [DefaultMetadataField]. Pass "" to flatten
	// metadata onto the document root (filters then address top-level
	// fields).
	MetadataField string

	// MetadataFieldsToFilter pre-declares the metadata keys that
	// should be indexed as filter fields in the Atlas search index.
	// Filtering on a metadata field requires the field to be listed
	// here when InitializeSchema is true.
	MetadataFieldsToFilter []string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher vectorstore.Batcher

	// Dimensions is the embedding width written into a new search-index
	// definition. When zero and InitializeSchema is true, the store probes
	// EmbeddingModel.
	Dimensions int

	// Similarity selects the vector similarity function. Optional:
	// defaults to [SimilarityCosine].
	Similarity Similarity

	// NumCandidates controls the recall/perf tradeoff of the Atlas
	// $vectorSearch stage. Optional: defaults to
	// [DefaultNumCandidates] (200).
	NumCandidates int

	// InitializeSchema, when true, creates the Atlas vector-search
	// index if it doesn't already exist. Requires a connected Atlas
	// cluster.
	InitializeSchema bool
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Collection == nil {
		return errors.New("mongodb: Collection is required")
	}
	if s.EmbeddingModel == nil {
		return errors.New("mongodb: EmbeddingModel is required")
	}
	if s.DocumentBatcher == nil {
		return errors.New("mongodb: DocumentBatcher is required")
	}
	if s.Dimensions < 0 {
		return errors.New("mongodb: Dimensions must be >= 0")
	}
	switch s.Similarity {
	case SimilarityCosine, SimilarityEuclidean, SimilarityDotProduct:
	default:
		return fmt.Errorf("mongodb: unsupported Similarity %q", s.Similarity)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (s *StoreConfig) applyDefaults() {
	s.VectorIndexName = cmp.Or(s.VectorIndexName, DefaultVectorIndexName)
	s.EmbeddingPath = cmp.Or(s.EmbeddingPath, DefaultEmbeddingPath)
	s.ContentField = cmp.Or(s.ContentField, DefaultContentField)
	s.MetadataField = cmp.Or(s.MetadataField, DefaultMetadataField)
	if s.NumCandidates <= 0 {
		s.NumCandidates = DefaultNumCandidates
	}
	s.Similarity = cmp.Or(s.Similarity, SimilarityCosine)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store implements vector-store capabilities with MongoDB Atlas Vector Search.
type Store struct {
	collection             *mongo.Collection
	vectorIndexName        string
	embeddingPath          string
	contentField           string
	metadataField          string
	metadataFieldsToFilter []string
	embeddingClient        embeddingclient.Client
	documentBatcher        vectorstore.Batcher
	dimensions             int
	similarity             Similarity
	numCandidates          int
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("mongodb: create embedding client: %w", err)
	}

	store := &Store{
		collection:             config.Collection,
		vectorIndexName:        config.VectorIndexName,
		embeddingPath:          config.EmbeddingPath,
		contentField:           config.ContentField,
		metadataField:          config.MetadataField,
		metadataFieldsToFilter: config.MetadataFieldsToFilter,
		embeddingClient:        embeddingClient,
		documentBatcher:        config.DocumentBatcher,
		dimensions:             config.Dimensions,
		similarity:             config.Similarity,
		numCandidates:          config.NumCandidates,
	}

	if err = store.initialize(ctx, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("mongodb: initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensionality and creates the Atlas vector
// index when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if !initSchema {
		return nil
	}
	if s.dimensions <= 0 {
		dimensions, err := s.embeddingClient.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("mongodb: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("mongodb: Dimensions must be > 0")
	}

	return s.createSearchIndex(ctx)
}

func (s *Store) createSearchIndex(ctx context.Context) error {
	cursor, err := s.collection.SearchIndexes().List(ctx, options.SearchIndexes().SetName(s.vectorIndexName))
	if err != nil {
		return fmt.Errorf("mongodb: list search index %q: %w", s.vectorIndexName, err)
	}
	defer cursor.Close(ctx)
	if cursor.Next(ctx) {
		return nil // already exists
	}
	if err := cursor.Err(); err != nil {
		return fmt.Errorf("mongodb: read search indexes: %w", err)
	}

	fields := []bson.M{
		{
			"type":          "vector",
			"path":          s.embeddingPath,
			"numDimensions": s.dimensions,
			"similarity":    string(s.similarity),
		},
	}
	for _, name := range s.metadataFieldsToFilter {
		path := name
		if s.metadataField != "" {
			path = s.metadataField + "." + name
		}
		fields = append(fields, bson.M{
			"type": "filter",
			"path": path,
		})
	}

	definition := bson.M{"fields": fields}
	model := mongo.SearchIndexModel{
		Definition: definition,
		Options:    options.SearchIndexes().SetName(s.vectorIndexName).SetType("vectorSearch"),
	}
	if _, err := s.collection.SearchIndexes().CreateOne(ctx, model); err != nil {
		return fmt.Errorf("createSearchIndexes: %w", err)
	}
	return nil
}

// Index embeds documents and bulk-upserts them by _id.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("mongodb.Store.Index: %w", err)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("mongodb: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("mongodb: embed documents: %w", err)
		}

		writes := make([]mongo.WriteModel, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("mongodb: decode metadata for %s: %w", id, err)
			}

			payload := bson.M{
				defaultIDField:  id,
				s.contentField:  doc.Text,
				s.embeddingPath: embedding.Float32Vector(vectors[i]),
			}
			if s.metadataField != "" {
				meta := metadataValues
				if meta == nil {
					meta = map[string]any{}
				}
				payload[s.metadataField] = meta
			} else {
				for k, v := range metadataValues {
					payload[k] = v
				}
			}

			writes = append(writes, mongo.NewReplaceOneModel().
				SetFilter(bson.M{defaultIDField: id}).
				SetReplacement(payload).
				SetUpsert(true),
			)
		}

		if _, err := s.collection.BulkWrite(ctx, writes); err != nil {
			return fmt.Errorf("mongodb: BulkWrite: %w", err)
		}
	}
	return nil
}

// Search runs the $vectorSearch aggregation and returns the matching
// documents above the configured MinScore threshold.
func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("mongodb.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("mongodb: embed query: %w", err)
	}
	queryVec := embedding.Float32Vector(vector)

	vectorSearch := bson.M{
		"index":         s.vectorIndexName,
		"path":          s.embeddingPath,
		"queryVector":   queryVec,
		"numCandidates": s.numCandidates,
		"limit":         req.Options.TopK,
	}
	if req.Options.Filter != nil {
		filterDoc, filterErr := s.buildFilter(req.Options.Filter)
		if filterErr != nil {
			return nil, filterErr
		}
		if len(filterDoc) > 0 {
			vectorSearch["filter"] = filterDoc
		}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$vectorSearch", Value: vectorSearch}},
		{{Key: "$addFields", Value: bson.M{
			scoreField: bson.M{"$meta": "vectorSearchScore"},
		}}},
	}
	if req.Options.MinScore > 0 {
		pipeline = append(pipeline, bson.D{
			{Key: "$match", Value: bson.M{scoreField: bson.M{"$gte": req.Options.MinScore}}},
		})
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongodb: aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	docs = make([]*vectorstore.SearchResult, 0, req.Options.TopK)
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("mongodb: decode hit: %w", err)
		}
		match, err := s.toMatch(raw)
		if err != nil {
			return nil, err
		}
		docs = append(docs, match)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongodb: cursor: %w", err)
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

// Delete removes documents matching the filter expression.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("mongodb.Store.DeleteWhere: %w", err)
	}

	var filter bson.M
	filter, err = s.buildFilter(expr)
	if err != nil {
		return err
	}
	if len(filter) == 0 {
		return errors.New("mongodb: refusing to delete on empty filter")
	}

	if _, err := s.collection.DeleteMany(ctx, filter); err != nil {
		return fmt.Errorf("mongodb: DeleteMany: %w", err)
	}
	return nil
}

// DeleteIDs removes documents by their _id — `DeleteMany({_id: {$in: ids}})`.
// An empty slice is a no-op; unknown ids are silently ignored (idempotent).
// Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	if _, err = s.collection.DeleteMany(ctx, bson.M{defaultIDField: bson.M{"$in": ids}}); err != nil {
		return fmt.Errorf("mongodb: DeleteMany by ids: %w", err)
	}
	return nil
}

// buildFilter runs the AST through the visitor and returns the
// MongoDB filter document.
func (s *Store) buildFilter(expr filter.Predicate) (bson.M, error) {
	if expr == nil {
		return nil, nil
	}
	v := NewVisitor(s.metadataField)
	if err := expr.Accept(v); err != nil {
		return nil, fmt.Errorf("mongodb: convert filter: %w", err)
	}
	return bson.M(v.Result()), nil
}

func (s *Store) toMatch(raw bson.M) (*vectorstore.SearchResult, error) {
	id, ok := raw[defaultIDField].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("mongodb: result is missing string field %q", defaultIDField)
	}
	content, ok := raw[s.contentField].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("mongodb: result is missing string field %q", s.contentField)
	}
	doc := &document.Document{ID: id, Text: content}
	var rawScore float64
	switch value := raw[scoreField].(type) {
	case float64:
		rawScore = value
	case float32:
		rawScore = float64(value)
	default:
		return nil, fmt.Errorf("mongodb: result score has type %T, want number", raw[scoreField])
	}
	score := vectorstore.ScoreFromValue(rawScore)

	if s.metadataField != "" {
		switch meta := raw[s.metadataField].(type) {
		case bson.M:
			var err error
			doc.Metadata, err = metadata.FromValues(map[string]any(meta))
			if err != nil {
				return nil, fmt.Errorf("mongodb: convert metadata: %w", err)
			}
		case map[string]any:
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("mongodb: convert metadata: %w", err)
			}
		}
	} else {
		meta := make(map[string]any, len(raw))
		for k, v := range raw {
			switch k {
			case defaultIDField, s.contentField, s.embeddingPath, scoreField:
				continue
			}
			meta[k] = v
		}
		if len(meta) > 0 {
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("mongodb: convert metadata: %w", err)
			}
		}
	}
	return &vectorstore.SearchResult{Document: doc, Score: score}, nil
}

func (s *Store) Close() error { return nil }
