package pgvector

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdmath "math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvec "github.com/pgvector/pgvector-go"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "PgVector"

const (
	DefaultSchemaName     = "public"
	DefaultTableName      = "vector_store"
	DefaultMetadataColumn = "metadata"
	DefaultIndexSuffix    = "_embedding_idx"

	// DefaultDimensions falls back to OpenAI's text-embedding-3-small
	// width when the embedding model can't report its own dim and
	// the caller didn't pass one. Spring AI uses the same value.
	DefaultDimensions = 1536
)

// DistanceMetric picks the pgvector operator used to compute the
// similarity score and the matching index opclass.
type DistanceMetric string

const (
	// DistanceCosine uses the `<=>` operator. Returns 1 - cosine
	// similarity, range [0, 2]. Lower is more similar. Index opclass:
	// vector_cosine_ops.
	DistanceCosine DistanceMetric = "cosine"

	// DistanceL2 uses the `<->` operator (Euclidean distance). Range
	// [0, ∞). Lower is more similar. Index opclass: vector_l2_ops.
	DistanceL2 DistanceMetric = "l2"

	// DistanceIP uses the `<#>` operator (negative inner product).
	// pgvector returns a negative number so ORDER BY ascending still
	// surfaces the closest first. Index opclass: vector_ip_ops.
	DistanceIP DistanceMetric = "ip"
)

// IndexType selects the ANN index built when [StoreConfig.InitializeSchema]
// is true.
type IndexType string

const (
	// IndexHNSW builds a hierarchical-navigable-small-world graph.
	// Best query performance, slower builds, more memory. Default.
	IndexHNSW IndexType = "hnsw"

	// IndexIVFFlat builds an inverted-file index. Faster builds and
	// less memory than HNSW, with lower recall/perf at query time.
	IndexIVFFlat IndexType = "ivfflat"

	// IndexNone skips index creation. pgvector falls back to exact
	// (sequential-scan) nearest-neighbor search, which is fine for
	// small tables.
	IndexNone IndexType = "none"
)

// StoreConfig contains configuration options for the pgvector store.
type StoreConfig struct {
	// Context is the context used for the initial CONNECTION + schema
	// bootstrap. Optional: defaults to context.Background().
	Context context.Context

	// Pool is the pgx connection pool. Required.
	Pool *pgxpool.Pool

	// SchemaName is the PostgreSQL schema that holds the vector
	// table. Optional: defaults to [DefaultSchemaName] ("public").
	SchemaName string

	// TableName is the table that stores documents and their
	// embeddings. Optional: defaults to [DefaultTableName]
	// ("vector_store").
	TableName string

	// IndexName overrides the ANN-index name generated for new
	// tables. Optional: defaults to "<TableName><DefaultIndexSuffix>"
	// when InitializeSchema is true.
	IndexName string

	// MetadataColumn is the jsonb column used for metadata filtering.
	// Optional: defaults to [DefaultMetadataColumn] ("metadata").
	MetadataColumn string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before insertion. Required.
	DocumentBatcher document.Batcher

	// Dimensions sets the vector width for new tables. When zero, the
	// store first asks the embedding model for its native
	// dimensionality and falls back to [DefaultDimensions] (1536) if
	// that fails. The value MUST agree with the column width if the
	// table already exists.
	Dimensions int

	// DistanceMetric selects the similarity operator. Optional:
	// defaults to [DistanceCosine].
	DistanceMetric DistanceMetric

	// IndexType selects the ANN index built when InitializeSchema is
	// true. Optional: defaults to [IndexHNSW].
	IndexType IndexType

	// InitializeSchema, when true, creates the extension, schema,
	// table, and ANN index if they don't already exist. When false,
	// the store assumes the schema is already provisioned.
	InitializeSchema bool

	// SkipExtensionCreate suppresses the `CREATE EXTENSION
	// IF NOT EXISTS vector` step run during initialize. Set to
	// true for CockroachDB and other Postgres-compatible engines
	// that ship VECTOR support natively and reject CREATE EXTENSION.
	SkipExtensionCreate bool
}

func (c *StoreConfig) Validate() error {
	if c.Pool == nil {
		return errors.New("pgvector: Pool is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("pgvector: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("pgvector: DocumentBatcher is required")
	}
	return ident.Check("pgvector", map[string]string{
		"SchemaName":     c.SchemaName,
		"TableName":      c.TableName,
		"IndexName":      c.IndexName,
		"MetadataColumn": c.MetadataColumn,
	})
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.SchemaName = cmp.Or(c.SchemaName, DefaultSchemaName)
	c.TableName = cmp.Or(c.TableName, DefaultTableName)
	c.MetadataColumn = cmp.Or(c.MetadataColumn, DefaultMetadataColumn)
	if c.IndexName == "" {
		c.IndexName = c.TableName + DefaultIndexSuffix
	}
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DistanceCosine)
	c.IndexType = cmp.Or(c.IndexType, IndexHNSW)
}

var (
	_ vectorstore.Store     = (*Store)(nil)
	_ vectorstore.IDDeleter = (*Store)(nil)
)

// Store is a pgvector-backed implementation of [vectorstore.Store].
type Store struct {
	pool                *pgxpool.Pool
	schemaName          string
	tableName           string
	indexName           string
	metadataColumn      string
	fullTable           string
	embeddingModel      embedding.Model
	embeddingClient     *embedding.Client
	documentBatcher     document.Batcher
	dimensions          int
	distanceMetric      DistanceMetric
	indexType           IndexType
	skipExtensionCreate bool
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("pgvector: failed to create embedding client: %w", err)
	}

	store := &Store{
		pool:                config.Pool,
		schemaName:          config.SchemaName,
		tableName:           config.TableName,
		indexName:           config.IndexName,
		metadataColumn:      config.MetadataColumn,
		fullTable:           config.SchemaName + "." + config.TableName,
		embeddingModel:      config.EmbeddingModel,
		embeddingClient:     embeddingClient,
		documentBatcher:     config.DocumentBatcher,
		dimensions:          config.Dimensions,
		distanceMetric:      config.DistanceMetric,
		indexType:           config.IndexType,
		skipExtensionCreate: config.SkipExtensionCreate,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("pgvector: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves the vector dimensionality and, when requested,
// provisions the schema, table, and ANN index.
func (s *Store) initialize(ctx context.Context, initializeSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("pgvector: Dimensions must be > 0")
	}

	if !initializeSchema {
		return nil
	}

	stmts := make([]string, 0, 4)
	if !s.skipExtensionCreate {
		stmts = append(stmts, `CREATE EXTENSION IF NOT EXISTS vector`)
	}
	stmts = append(stmts, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, s.schemaName))
	stmts = append(stmts, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id text PRIMARY KEY,
			content text,
			%s jsonb,
			embedding vector(%d)
		)
	`, s.fullTable, s.metadataColumn, s.dimensions))

	if s.indexType != IndexNone {
		stmts = append(stmts, fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s USING %s (embedding %s)`,
			s.indexName, s.fullTable, s.indexType, s.distanceMetric.indexOpClass(),
		))
	}

	for _, stmt := range stmts {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("execute %q: %w", strings.SplitN(stmt, "\n", 2)[0], err)
		}
	}
	return nil
}

// indexOpClass maps a [DistanceMetric] onto the matching pgvector
// index opclass name.
func (d DistanceMetric) indexOpClass() string {
	switch d {
	case DistanceL2:
		return "vector_l2_ops"
	case DistanceIP:
		return "vector_ip_ops"
	case DistanceCosine:
		fallthrough
	default:
		return "vector_cosine_ops"
	}
}

// operator returns the pgvector binary operator used by ORDER BY for
// this distance metric.
func (d DistanceMetric) operator() string {
	switch d {
	case DistanceL2:
		return "<->"
	case DistanceIP:
		return "<#>"
	case DistanceCosine:
		fallthrough
	default:
		return "<=>"
	}
}

// distanceToScore maps the raw distance returned by pgvector onto a
// "higher = more similar" score in [0, 1], matching the rest of the
// lynx vectorstore providers.
func (s *Store) distanceToScore(distance float64) float64 {
	switch s.distanceMetric {
	case DistanceL2:
		// L2 is unbounded; squash into (0, 1].
		return 1.0 / (1.0 + distance)
	case DistanceIP:
		// pgvector's <#> returns -(inner_product). Negate so higher
		// IP → higher score, then squash through sigmoid so the
		// final value stays in (0, 1).
		ip := -distance
		return 1.0 / (1.0 + stdmath.Exp(-ip))
	case DistanceCosine:
		fallthrough
	default:
		// Cosine distance ∈ [0, 2]; (1 - d/2) collapses it to [0, 1].
		// Clamp to dodge floating-point overshoot.
		score := 1.0 - distance/2.0
		switch {
		case score < 0:
			return 0
		case score > 1:
			return 1
		default:
			return score
		}
	}
}

// Create embeds the documents in req and upserts them into the
// pgvector table.
//
// One `db.vector.create pgvector` span per call carrying
// `db.system` / `db.operation.name` / `rag.doc_count`.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("pgvector: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "pgvector", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("pgvector: failed to batch documents: %w", err)
	}

	upsertSQL := fmt.Sprintf(
		`INSERT INTO %s (id, content, %s, embedding) VALUES ($1, $2, $3::jsonb, $4)
		 ON CONFLICT (id) DO UPDATE SET
		   content   = EXCLUDED.content,
		   %s        = EXCLUDED.%s,
		   embedding = EXCLUDED.embedding`,
		s.fullTable, s.metadataColumn, s.metadataColumn, s.metadataColumn,
	)

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("pgvector: failed to generate embeddings: %w", err)
		}

		batch := &pgx.Batch{}
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}

			metaJSON, err := marshalMetadata(doc.Metadata)
			if err != nil {
				return fmt.Errorf("pgvector: marshal metadata for document %s: %w", id, err)
			}

			vec := pgvec.NewVector(math.ConvertSlice[float64, float32](vectors[i]))
			batch.Queue(upsertSQL, id, doc.Text, metaJSON, vec)
		}

		results := s.pool.SendBatch(ctx, batch)
		execErr := drainBatch(results, len(docs))
		closeErr := results.Close()
		if execErr != nil {
			return fmt.Errorf("pgvector: upsert batch: %w", execErr)
		}
		if closeErr != nil {
			return fmt.Errorf("pgvector: close upsert batch: %w", closeErr)
		}
	}
	return nil
}

// drainBatch consumes every queued statement's tag so the underlying
// connection isn't left in an inconsistent state on close.
func drainBatch(br pgx.BatchResults, n int) error {
	for range n {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// Retrieve embeds the query, runs an ANN search, and returns the
// matching documents above the configured MinScore threshold.
//
// One `db.vector.retrieve pgvector` span per call carrying
// `db.vector.query.top_k` / `db.vector.query.similarity_threshold`
// and (on success) `rag.doc_count`.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "pgvector", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("pgvector: failed to embed query: %w", err)
	}
	queryVec := pgvec.NewVector(math.ConvertSlice[float64, float32](vector))

	whereSQL, args, err := s.buildWhereClause(req.Filter)
	if err != nil {
		return nil, err
	}

	args = append(args, queryVec)
	distancePlaceholder := fmt.Sprintf("$%d", len(args))
	args = append(args, req.TopK)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))

	sql := fmt.Sprintf(
		`SELECT id, content, %s, embedding %s %s AS distance FROM %s%s ORDER BY distance LIMIT %s`,
		s.metadataColumn, s.distanceMetric.operator(), distancePlaceholder,
		s.fullTable, whereSQL, limitPlaceholder,
	)

	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("pgvector: query %s: %w", s.fullTable, err)
	}
	defer rows.Close()

	docs = make([]*document.Document, 0, req.TopK)
	for rows.Next() {
		var (
			id       string
			content  *string
			metaRaw  []byte
			distance float64
		)
		if err = rows.Scan(&id, &content, &metaRaw, &distance); err != nil {
			return nil, fmt.Errorf("pgvector: scan row: %w", err)
		}

		score := s.distanceToScore(distance)
		if score < req.MinScore {
			continue
		}

		doc := &document.Document{ID: id, Score: score}
		if content != nil {
			doc.Text = *content
		}
		if len(metaRaw) > 0 {
			if doc.Metadata, err = unmarshalMetadata(metaRaw); err != nil {
				return nil, fmt.Errorf("pgvector: unmarshal metadata for %s: %w", id, err)
			}
		}
		docs = append(docs, doc)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("pgvector: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes every row whose metadata matches the request filter.
//
// One `db.vector.delete pgvector` span per call.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("pgvector: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "pgvector")
	defer func() { tracing.Finish(span, err) }()

	var (
		fragment string
		args     []any
	)
	fragment, args, err = s.buildWhereClause(req.Filter)
	if err != nil {
		return err
	}
	if fragment == "" {
		return errors.New("pgvector: empty filter produced no WHERE clause")
	}

	sql := fmt.Sprintf(`DELETE FROM %s%s`, s.fullTable, fragment)
	if _, err = s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("pgvector: delete from %s: %w", s.fullTable, err)
	}
	return nil
}

// DeleteByIDs removes rows by primary key — `DELETE ... WHERE id = ANY($1)`.
// pgx maps the []string to a Postgres text array. An empty slice is a
// no-op; unknown ids are silently ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "pgvector")
	defer func() { tracing.Finish(span, err) }()

	sql := fmt.Sprintf(`DELETE FROM %s WHERE id = ANY($1)`, s.fullTable)
	if _, err = s.pool.Exec(ctx, sql, ids); err != nil {
		return fmt.Errorf("pgvector: delete by ids from %s: %w", s.fullTable, err)
	}
	return nil
}

// buildWhereClause converts the optional filter expression into a SQL
// fragment (prefixed with " WHERE ") and the matching argument slice.
// Returns ("", nil, nil) when filter is nil.
func (s *Store) buildWhereClause(filter ast.Expr) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	visitor := NewVisitor(s.metadataColumn)
	visitor.Visit(filter)
	if err := visitor.Error(); err != nil {
		return "", nil, fmt.Errorf("pgvector: convert filter: %w", err)
	}
	fragment, args := visitor.Result()
	if fragment == "" {
		return "", nil, nil
	}
	return " WHERE " + fragment, args, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.pool,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

// marshalMetadata serializes the document metadata into the JSON bytes
// stored in the jsonb column. nil maps round-trip as JSON null.
func marshalMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m)
}

// unmarshalMetadata reverses marshalMetadata. NULL jsonb columns
// produce a nil map.
func unmarshalMetadata(b []byte) (map[string]any, error) {
	if len(b) == 0 || string(b) == "null" {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
