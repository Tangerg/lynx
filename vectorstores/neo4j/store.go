package neo4j

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "Neo4j"

const (
	DefaultLabel             = "Document"
	DefaultIndexName         = "lynx-vector-index"
	DefaultEmbeddingProperty = "embedding"
	DefaultIDProperty        = "id"
	DefaultTextProperty      = "text"
	DefaultMetadataPrefix    = "metadata"
	DefaultDimensions        = 1536
)

// SimilarityFunction selects the function written into the vector
// index definition. The chosen value is recorded at index creation
// time and cannot be changed without rebuilding the index.
type SimilarityFunction string

const (
	// SimilarityCosine — cosine similarity. Default.
	SimilarityCosine SimilarityFunction = "cosine"

	// SimilarityEuclidean — Euclidean distance, mapped to a [0, 1]
	// similarity score by Neo4j itself.
	SimilarityEuclidean SimilarityFunction = "euclidean"
)

// safeIdentifier matches the standard SQL unquoted identifier shape.
// We use it to validate caller-supplied label / property / index
// names that are interpolated into Cypher DDL.

// StoreConfig contains configuration options for the Neo4j vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// Driver is the Neo4j context-aware driver instance. Required.
	Driver neo4j.DriverWithContext

	// Database is the Neo4j database name. Optional: defaults to the
	// driver's default database (typically "neo4j").
	Database string

	// Label is the node label used for documents. Optional: defaults
	// to [DefaultLabel].
	Label string

	// IndexName is the vector index name. Optional: defaults to
	// [DefaultIndexName].
	IndexName string

	// EmbeddingProperty is the node property that stores the vector.
	// Optional: defaults to [DefaultEmbeddingProperty].
	EmbeddingProperty string

	// IDProperty is the node property that stores the document id.
	// Optional: defaults to [DefaultIDProperty].
	IDProperty string

	// TextProperty is the node property that stores the document
	// text. Optional: defaults to [DefaultTextProperty].
	TextProperty string

	// MetadataPrefix is the property-name prefix used for metadata
	// keys (so "metadata.author" instead of "author"). Optional:
	// defaults to [DefaultMetadataPrefix]. Pass "" to write metadata
	// keys as top-level properties.
	MetadataPrefix string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher document.Batcher

	// Dimensions sets the vector width recorded in the index
	// definition. When zero, falls back to the embedding model's
	// reported value and then [DefaultDimensions].
	Dimensions int

	// Similarity selects the vector similarity function. Optional:
	// defaults to [SimilarityCosine].
	Similarity SimilarityFunction

	// InitializeSchema, when true, creates the unique-id constraint
	// and the vector index if they don't already exist.
	InitializeSchema bool
}

func (c *StoreConfig) Validate() error {
	if c.Driver == nil {
		return errors.New("neo4j: Driver is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("neo4j: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("neo4j: DocumentBatcher is required")
	}
	return ident.Check("neo4j", map[string]string{
		"Label":             c.Label,
		"EmbeddingProperty": c.EmbeddingProperty,
		"IDProperty":        c.IDProperty,
		"TextProperty":      c.TextProperty,
	})
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.Label = cmp.Or(c.Label, DefaultLabel)
	c.IndexName = cmp.Or(c.IndexName, DefaultIndexName)
	c.EmbeddingProperty = cmp.Or(c.EmbeddingProperty, DefaultEmbeddingProperty)
	c.IDProperty = cmp.Or(c.IDProperty, DefaultIDProperty)
	c.TextProperty = cmp.Or(c.TextProperty, DefaultTextProperty)
	c.MetadataPrefix = cmp.Or(c.MetadataPrefix, DefaultMetadataPrefix)
	c.Similarity = cmp.Or(c.Similarity, SimilarityCosine)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Neo4j-backed [vectorstore.Store] implementation. Each
// document maps onto a node carrying the configured label and a flat
// set of metadata properties.
type Store struct {
	driver            neo4j.DriverWithContext
	database          string
	label             string
	indexName         string
	embeddingProperty string
	idProperty        string
	textProperty      string
	metadataPrefix    string
	embeddingModel    embedding.Model
	embeddingClient   *embedding.Client
	documentBatcher   document.Batcher
	dimensions        int
	similarity        SimilarityFunction
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("neo4j: failed to create embedding client: %w", err)
	}

	store := &Store{
		driver:            config.Driver,
		database:          config.Database,
		label:             config.Label,
		indexName:         config.IndexName,
		embeddingProperty: config.EmbeddingProperty,
		idProperty:        config.IDProperty,
		textProperty:      config.TextProperty,
		metadataPrefix:    config.MetadataPrefix,
		embeddingModel:    config.EmbeddingModel,
		embeddingClient:   embeddingClient,
		documentBatcher:   config.DocumentBatcher,
		dimensions:        config.Dimensions,
		similarity:        config.Similarity,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("neo4j: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensionality and provisions the vector index
// when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("neo4j: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}

	constraintName := s.indexName + "_unique"
	constraintStmt := fmt.Sprintf(
		"CREATE CONSTRAINT `%s` IF NOT EXISTS FOR (n:`%s`) REQUIRE n.`%s` IS UNIQUE",
		constraintName, s.label, s.idProperty,
	)
	indexStmt := fmt.Sprintf(
		"CREATE VECTOR INDEX `%s` IF NOT EXISTS FOR (n:`%s`) ON (n.`%s`) "+
			"OPTIONS {indexConfig: {`vector.dimensions`: %d, `vector.similarity_function`: '%s'}}",
		s.indexName, s.label, s.embeddingProperty, s.dimensions, s.similarity,
	)

	return s.write(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		if _, err := tx.Run(ctx, constraintStmt, nil); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, indexStmt, nil); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

// session opens a session bound to the configured database, if any.
func (s *Store) session(ctx context.Context, accessMode neo4j.AccessMode) neo4j.SessionWithContext {
	cfg := neo4j.SessionConfig{AccessMode: accessMode}
	if s.database != "" {
		cfg.DatabaseName = s.database
	}
	return s.driver.NewSession(ctx, cfg)
}

// write runs work inside a managed write transaction.
func (s *Store) write(ctx context.Context, work neo4j.ManagedTransactionWork) error {
	session := s.session(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)
	if _, err := session.ExecuteWrite(ctx, work); err != nil {
		return err
	}
	return nil
}

// Create embeds documents and upserts them as nodes.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("neo4j: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "neo4j", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("neo4j: failed to batch documents: %w", err)
	}

	upsertCypher := fmt.Sprintf(
		"UNWIND $rows AS row "+
			"MERGE (n:`%s` {`%s`: row.id}) "+
			"SET n += row.properties "+
			"WITH row, n "+
			"CALL db.create.setNodeVectorProperty(n, $embeddingProperty, row.embedding) "+
			"RETURN count(*)",
		s.label, s.idProperty,
	)

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("neo4j: failed to generate embeddings: %w", err)
		}

		rows := make([]map[string]any, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			rows = append(rows, map[string]any{
				"id":         id,
				"properties": s.documentProperties(doc),
				"embedding":  math.ConvertSlice[float64, float32](vectors[i]),
			})
		}

		if err := s.write(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, err := tx.Run(ctx, upsertCypher, map[string]any{
				"rows":              rows,
				"embeddingProperty": s.embeddingProperty,
			})
			return nil, err
		}); err != nil {
			return fmt.Errorf("neo4j: upsert: %w", err)
		}
	}
	return nil
}

// documentProperties assembles the property map written onto the
// upserted node — text plus metadata, with metadata keys optionally
// flattened under the configured prefix.
func (s *Store) documentProperties(doc *document.Document) map[string]any {
	props := make(map[string]any, len(doc.Metadata)+1)
	props[s.textProperty] = doc.Text
	prefix := ""
	if s.metadataPrefix != "" {
		prefix = s.metadataPrefix + "."
	}
	for k, v := range doc.Metadata {
		props[prefix+k] = v
	}
	return props
}

// Retrieve calls db.index.vector.queryNodes and returns matching
// documents above MinScore.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("neo4j: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "neo4j", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("neo4j: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	wherePredicate, params, err := s.buildPredicate(req.Filter)
	if err != nil {
		return nil, err
	}

	whereClause := "score >= $threshold"
	if wherePredicate != "" {
		whereClause = whereClause + " AND " + wherePredicate
	}

	cypher := fmt.Sprintf(
		"CALL db.index.vector.queryNodes($indexName, $k, $vec) YIELD node, score "+
			"WHERE %s RETURN node, score",
		whereClause,
	)

	if params == nil {
		params = make(map[string]any, 4)
	}
	params["indexName"] = s.indexName
	params["k"] = req.TopK
	params["vec"] = queryVec
	params["threshold"] = req.MinScore

	session := s.session(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	var result any
	result, err = session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		records, err := res.Collect(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]*document.Document, 0, len(records))
		for _, rec := range records {
			doc, err := s.recordToDocument(rec)
			if err != nil {
				return nil, err
			}
			out = append(out, doc)
		}
		return out, nil
	})
	if err != nil {
		return nil, fmt.Errorf("neo4j: vector query: %w", err)
	}
	docs = result.([]*document.Document)
	return docs, nil
}

func (s *Store) recordToDocument(rec *neo4j.Record) (*document.Document, error) {
	nodeRaw, found := rec.Get("node")
	if !found {
		return nil, errors.New("neo4j: result record missing 'node' field")
	}
	node, ok := nodeRaw.(neo4j.Node)
	if !ok {
		return nil, fmt.Errorf("neo4j: unexpected node type %T", nodeRaw)
	}

	var score float64
	if v, found := rec.Get("score"); found {
		switch sv := v.(type) {
		case float64:
			score = sv
		case float32:
			score = float64(sv)
		case int64:
			score = float64(sv)
		}
	}

	doc := &document.Document{Score: score}
	if id, ok := node.Props[s.idProperty].(string); ok {
		doc.ID = id
	}
	if text, ok := node.Props[s.textProperty].(string); ok {
		doc.Text = text
	}

	if len(node.Props) > 0 {
		prefix := ""
		if s.metadataPrefix != "" {
			prefix = s.metadataPrefix + "."
		}
		meta := make(map[string]any)
		for k, v := range node.Props {
			switch k {
			case s.idProperty, s.textProperty, s.embeddingProperty:
				continue
			}
			if prefix != "" && strings.HasPrefix(k, prefix) {
				meta[strings.TrimPrefix(k, prefix)] = v
			} else if prefix == "" {
				meta[k] = v
			}
		}
		if len(meta) > 0 {
			doc.Metadata = meta
		}
	}
	return doc, nil
}

// Delete removes nodes matching the filter expression.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("neo4j: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "neo4j")
	defer func() { tracing.Finish(span, err) }()

	predicate, params, err := s.buildPredicate(req.Filter)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("neo4j: refusing to delete on empty filter")
	}

	cypher := fmt.Sprintf(
		"MATCH (node:`%s`) WHERE %s DETACH DELETE node",
		s.label, predicate,
	)

	return s.write(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, cypher, params)
		return nil, err
	})
}

// buildPredicate converts the optional filter into a Cypher WHERE
// fragment plus its parameter bindings. Returns ("", nil, nil) when
// filter is nil.
func (s *Store) buildPredicate(filter ast.Expr) (string, map[string]any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor("node", s.metadataPrefix)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", nil, fmt.Errorf("neo4j: convert filter: %w", err)
	}
	predicate, params := v.Result()
	return predicate, params, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.driver,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }
