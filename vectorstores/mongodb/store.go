package mongodb

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "MongoDB"

const (
	DefaultVectorIndexName = "vector_index"
	DefaultEmbeddingPath   = "embedding"
	DefaultContentField    = "content"
	DefaultMetadataField   = "metadata"
	DefaultNumCandidates   = 200
	DefaultDimensions      = 1536
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
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

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
	DocumentBatcher document.Batcher

	// Dimensions is the embedding width written into the search index
	// definition. When zero, falls back to the embedding model's
	// reported value and then [DefaultDimensions].
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

func (c *StoreConfig) Validate() error {
	if c.Collection == nil {
		return errors.New("mongodb: Collection is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("mongodb: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("mongodb: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.VectorIndexName = cmp.Or(c.VectorIndexName, DefaultVectorIndexName)
	c.EmbeddingPath = cmp.Or(c.EmbeddingPath, DefaultEmbeddingPath)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.MetadataField = cmp.Or(c.MetadataField, DefaultMetadataField)
	if c.NumCandidates <= 0 {
		c.NumCandidates = DefaultNumCandidates
	}
	c.Similarity = cmp.Or(c.Similarity, SimilarityCosine)
}

var (
	_ vectorstore.Store     = (*Store)(nil)
	_ vectorstore.IDDeleter = (*Store)(nil)
)

// Store is a MongoDB Atlas Vector Search backed [vectorstore.Store].
type Store struct {
	collection             *mongo.Collection
	vectorIndexName        string
	embeddingPath          string
	contentField           string
	metadataField          string
	metadataFieldsToFilter []string
	embeddingModel         embedding.Model
	embeddingClient        *embedding.Client
	documentBatcher        document.Batcher
	dimensions             int
	similarity             Similarity
	numCandidates          int
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to create embedding client: %w", err)
	}

	store := &Store{
		collection:             config.Collection,
		vectorIndexName:        config.VectorIndexName,
		embeddingPath:          config.EmbeddingPath,
		contentField:           config.ContentField,
		metadataField:          config.MetadataField,
		metadataFieldsToFilter: config.MetadataFieldsToFilter,
		embeddingModel:         config.EmbeddingModel,
		embeddingClient:        embeddingClient,
		documentBatcher:        config.DocumentBatcher,
		dimensions:             config.Dimensions,
		similarity:             config.Similarity,
		numCandidates:          config.NumCandidates,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("mongodb: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensionality and creates the Atlas vector
// index when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("mongodb: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}
	return s.createSearchIndex(ctx)
}

func (s *Store) createSearchIndex(ctx context.Context) error {
	cursor, err := s.collection.SearchIndexes().List(ctx, options.SearchIndexes().SetName(s.vectorIndexName))
	if err == nil {
		defer cursor.Close(ctx)
		if cursor.Next(ctx) {
			return nil // already exists
		}
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

// Create embeds documents and bulk-upserts them by _id.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("mongodb: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "mongodb", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("mongodb: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("mongodb: failed to generate embeddings: %w", err)
		}

		writes := make([]mongo.WriteModel, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}

			payload := bson.M{
				defaultIDField:  id,
				s.contentField:  doc.Text,
				s.embeddingPath: math.ConvertSlice[float64, float32](vectors[i]),
			}
			if s.metadataField != "" {
				meta := doc.Metadata
				if meta == nil {
					meta = map[string]any{}
				}
				payload[s.metadataField] = meta
			} else {
				for k, v := range doc.Metadata {
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

// Retrieve runs the $vectorSearch aggregation and returns the matching
// documents above the configured MinScore threshold.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("mongodb: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "mongodb", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("mongodb: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	vectorSearch := bson.M{
		"index":         s.vectorIndexName,
		"path":          s.embeddingPath,
		"queryVector":   queryVec,
		"numCandidates": s.numCandidates,
		"limit":         req.TopK,
	}
	if req.Filter != nil {
		filterDoc, filterErr := s.buildFilter(req.Filter)
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
	if req.MinScore > 0 {
		pipeline = append(pipeline, bson.D{
			{Key: "$match", Value: bson.M{scoreField: bson.M{"$gte": req.MinScore}}},
		})
	}

	cursor, err := s.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("mongodb: aggregate: %w", err)
	}
	defer cursor.Close(ctx)

	docs = make([]*document.Document, 0, req.TopK)
	for cursor.Next(ctx) {
		var raw bson.M
		if err := cursor.Decode(&raw); err != nil {
			return nil, fmt.Errorf("mongodb: decode hit: %w", err)
		}
		docs = append(docs, s.toDocument(raw))
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongodb: cursor: %w", err)
	}
	return docs, nil
}

// Delete removes documents matching the filter expression.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("mongodb: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "mongodb")
	defer func() { tracing.Finish(span, err) }()

	var filter bson.M
	filter, err = s.buildFilter(req.Filter)
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

// DeleteByIDs removes documents by their _id — `DeleteMany({_id: {$in: ids}})`.
// An empty slice is a no-op; unknown ids are silently ignored (idempotent).
// Implements [vectorstore.IDDeleter].
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "mongodb")
	defer func() { tracing.Finish(span, err) }()

	if _, err = s.collection.DeleteMany(ctx, bson.M{defaultIDField: bson.M{"$in": ids}}); err != nil {
		return fmt.Errorf("mongodb: DeleteMany by ids: %w", err)
	}
	return nil
}

// buildFilter runs the AST through the visitor and returns the
// MongoDB filter document.
func (s *Store) buildFilter(expr ast.Expr) (bson.M, error) {
	if expr == nil {
		return nil, nil
	}
	v := NewVisitor(s.metadataField)
	v.Visit(expr)
	if err := v.Error(); err != nil {
		return nil, fmt.Errorf("mongodb: convert filter: %w", err)
	}
	return bson.M(v.Result()), nil
}

func (s *Store) toDocument(raw bson.M) *document.Document {
	doc := &document.Document{}
	if id, ok := raw[defaultIDField].(string); ok {
		doc.ID = id
	}
	if content, ok := raw[s.contentField].(string); ok {
		doc.Text = content
	}
	switch sv := raw[scoreField].(type) {
	case float64:
		doc.Score = sv
	case float32:
		doc.Score = float64(sv)
	case int32:
		doc.Score = float64(sv)
	case int64:
		doc.Score = float64(sv)
	}

	if s.metadataField != "" {
		switch meta := raw[s.metadataField].(type) {
		case bson.M:
			doc.Metadata = map[string]any(meta)
		case map[string]any:
			doc.Metadata = meta
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
			doc.Metadata = meta
		}
	}
	return doc
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.collection,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }
