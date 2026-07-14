package tidb

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "TiDB"

const (
	DefaultTableName       = "vector_store"
	DefaultIDColumn        = "id"
	DefaultContentColumn   = "content"
	DefaultMetadataColumn  = "metadata"
	DefaultEmbeddingColumn = "embedding"
	DefaultDimensions      = 1536
	DefaultDistanceMetric  = DistanceCosine
)

// DistanceMetric selects the VEC_*_DISTANCE function used at query
// time.
type DistanceMetric string

const (
	DistanceCosine     DistanceMetric = "COSINE"
	DistanceL2         DistanceMetric = "L2"
	DistanceNegativeIP DistanceMetric = "NEGATIVE_INNER_PRODUCT"
)

// safeIdentifier matches the standard SQL unquoted identifier shape.

// StoreConfig contains configuration options for the TiDB Vector
// store (TiDB 7.4+ with vector support enabled).
type StoreConfig struct {
	Context context.Context

	// DB is the database handle. Required. Use a *sql.DB built from
	// github.com/go-sql-driver/mysql pointed at a TiDB cluster.
	DB *sql.DB

	SchemaName      string
	TableName       string
	IDColumn        string
	ContentColumn   string
	MetadataColumn  string
	EmbeddingColumn string

	EmbeddingModel  embedding.Model
	DocumentBatcher vectorstores.Batcher

	Dimensions       int
	DistanceMetric   DistanceMetric
	InitializeSchema bool
}

func (c *StoreConfig) Validate() error {
	if c.DB == nil {
		return errors.New("tidb: DB is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("tidb: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("tidb: DocumentBatcher is required")
	}
	checks := map[string]string{
		"TableName":       c.TableName,
		"IDColumn":        c.IDColumn,
		"ContentColumn":   c.ContentColumn,
		"MetadataColumn":  c.MetadataColumn,
		"EmbeddingColumn": c.EmbeddingColumn,
	}
	if c.SchemaName != "" {
		checks["SchemaName"] = c.SchemaName
	}
	return ident.Check("tidb", checks)
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.TableName = cmp.Or(c.TableName, DefaultTableName)
	c.IDColumn = cmp.Or(c.IDColumn, DefaultIDColumn)
	c.ContentColumn = cmp.Or(c.ContentColumn, DefaultContentColumn)
	c.MetadataColumn = cmp.Or(c.MetadataColumn, DefaultMetadataColumn)
	c.EmbeddingColumn = cmp.Or(c.EmbeddingColumn, DefaultEmbeddingColumn)
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DefaultDistanceMetric)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store is a TiDB-backed the vectorstore capability interfaces implementation using
// TiDB's native VECTOR column type and VEC_*_DISTANCE functions.
type Store struct {
	db              *sql.DB
	schemaName      string
	tableName       string
	fullTable       string
	idColumn        string
	contentColumn   string
	metadataColumn  string
	embeddingColumn string
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher vectorstores.Batcher
	dimensions      int
	distanceMetric  DistanceMetric
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("tidb: failed to create embedding client: %w", err)
	}
	fullTable := config.TableName
	if config.SchemaName != "" {
		fullTable = config.SchemaName + "." + config.TableName
	}
	store := &Store{
		db:              config.DB,
		schemaName:      config.SchemaName,
		tableName:       config.TableName,
		fullTable:       fullTable,
		idColumn:        config.IDColumn,
		contentColumn:   config.ContentColumn,
		metadataColumn:  config.MetadataColumn,
		embeddingColumn: config.EmbeddingColumn,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
		distanceMetric:  config.DistanceMetric,
	}
	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("tidb: failed to initialize store: %w", err)
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		dimensions, err := embedding.ResolveDimensions(ctx, s.embeddingModel)
		if err != nil {
			return fmt.Errorf("tidb: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("tidb: Dimensions must be > 0")
	}
	if !initSchema {
		return nil
	}

	stmt := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			%s VARCHAR(64) NOT NULL PRIMARY KEY,
			%s TEXT,
			%s JSON,
			%s VECTOR(%d) NOT NULL
		)`,
		s.fullTable, s.idColumn, s.contentColumn, s.metadataColumn,
		s.embeddingColumn, s.dimensions,
	)
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("create table %s: %w", s.fullTable, err)
	}

	// TiDB's vector ANN index requires the function expression form.
	idxStmt := fmt.Sprintf(
		`CREATE VECTOR INDEX IF NOT EXISTS %s_idx ON %s ((%s(%s))) USING HNSW`,
		s.tableName, s.fullTable, distanceFunc(s.distanceMetric), s.embeddingColumn,
	)
	if _, err := s.db.ExecContext(ctx, idxStmt); err != nil {
		// Older TiDB versions may not yet support the HNSW vector
		// index; the table itself still works for exact search.
		// Surface the error so callers know the index didn't take.
		return fmt.Errorf("create vector index on %s: %w", s.fullTable, err)
	}
	return nil
}

func distanceFunc(metric DistanceMetric) string {
	switch metric {
	case DistanceL2:
		return "VEC_L2_DISTANCE"
	case DistanceNegativeIP:
		return "VEC_NEGATIVE_INNER_PRODUCT"
	case DistanceCosine:
		fallthrough
	default:
		return "VEC_COSINE_DISTANCE"
	}
}

// Add embeds documents and upserts them.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "tidb", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("tidb: failed to batch documents: %w", err)
	}

	upsert := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, ?) `+
			`ON DUPLICATE KEY UPDATE %s = VALUES(%s), %s = VALUES(%s), %s = VALUES(%s)`,
		s.fullTable, s.idColumn, s.contentColumn, s.metadataColumn, s.embeddingColumn,
		s.contentColumn, s.contentColumn,
		s.metadataColumn, s.metadataColumn,
		s.embeddingColumn, s.embeddingColumn,
	)

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("tidb: failed to generate embeddings: %w", err)
		}

		stmt, err := s.db.PrepareContext(ctx, upsert)
		if err != nil {
			return fmt.Errorf("tidb: prepare upsert: %w", err)
		}
		execErr := func() error {
			defer stmt.Close()
			for i, doc := range docs {
				id := doc.ID
				if id == "" {
					id = uuid.NewString()
				}
				metaJSON, err := marshalMetadata(doc.Metadata)
				if err != nil {
					return fmt.Errorf("marshal metadata for %s: %w", id, err)
				}
				vec32 := math.ConvertSlice[float64, float32](vectors[i])
				if _, err := stmt.ExecContext(ctx, id, doc.Text, metaJSON, docio.FormatVectorLiteral(vec32)); err != nil {
					return fmt.Errorf("upsert %s: %w", id, err)
				}
			}
			return nil
		}()
		if execErr != nil {
			return execErr
		}
	}
	return nil
}

// Search runs an ANN search ordered by the configured distance
// function.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("tidb: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "tidb", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("tidb: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)
	vecText := docio.FormatVectorLiteral(queryVec)

	wherePredicate, whereArgs, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}
	wherePart := ""
	if wherePredicate != "" {
		wherePart = " AND " + wherePredicate
	}

	distExpr := fmt.Sprintf("%s(%s, ?)", distanceFunc(s.distanceMetric), s.embeddingColumn)
	stmt := fmt.Sprintf(
		`SELECT %s, %s, %s, %s AS distance FROM %s WHERE 1=1%s ORDER BY distance ASC LIMIT ?`,
		s.idColumn, s.contentColumn, s.metadataColumn, distExpr,
		s.fullTable, wherePart,
	)

	args := make([]any, 0, len(whereArgs)+2)
	args = append(args, vecText)
	args = append(args, whereArgs...)
	args = append(args, req.TopK)

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("tidb: query %s: %w", s.fullTable, err)
	}
	defer rows.Close()

	docs = make([]vectorstore.Match, 0, req.TopK)
	for rows.Next() {
		var (
			id       string
			content  sql.NullString
			metaRaw  sql.NullString
			distance float64
		)
		if err = rows.Scan(&id, &content, &metaRaw, &distance); err != nil {
			return nil, fmt.Errorf("tidb: scan row: %w", err)
		}
		score := distanceToScore(s.distanceMetric, distance)
		if score < req.MinScore {
			continue
		}
		doc := &document.Document{ID: id}
		if content.Valid {
			doc.Text = content.String
		}
		if metaRaw.Valid {
			if doc.Metadata, err = unmarshalMetadata([]byte(metaRaw.String)); err != nil {
				return nil, fmt.Errorf("tidb: unmarshal metadata for %s: %w", id, err)
			}
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tidb: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes rows matching the filter expression.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "tidb")
	defer func() { tracing.Finish(span, err) }()

	var (
		predicate string
		args      []any
	)
	predicate, args, err = s.buildFilter(expr)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("tidb: refusing to delete on empty filter")
	}
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", s.fullTable, predicate)
	if _, err := s.db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("tidb: delete from %s: %w", s.fullTable, err)
	}
	return nil
}

// DeleteIDs removes rows by primary key —
// `DELETE ... WHERE <idCol> IN (?, ...)` with one placeholder per id.
// An empty slice is a no-op; unknown ids are silently ignored
// (idempotent). Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "tidb")
	defer func() { tracing.Finish(span, err) }()

	placeholders := strings.Repeat("?, ", len(ids)-1) + "?"
	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", s.fullTable, s.idColumn, placeholders)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if _, err = s.db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("tidb: delete by ids from %s: %w", s.fullTable, err)
	}
	return nil
}

func (s *Store) buildFilter(filter filter.Expr) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor(s.metadataColumn)
	if err := v.Visit(filter); err != nil {
		return "", nil, fmt.Errorf("tidb: convert filter: %w", err)
	}
	predicate, args := v.Result()
	return predicate, args, nil
}

func (s *Store) Close() error { return nil }

// distanceToScore maps a VEC_*_DISTANCE result onto a [0, 1]
// similarity score.
func distanceToScore(metric DistanceMetric, distance float64) float64 {
	switch metric {
	case DistanceL2:
		return 1.0 / (1.0 + distance)
	case DistanceNegativeIP:
		// TiDB returns -inner_product; recover ip and sigmoid to [0, 1].
		ip := -distance
		score := (ip + 1) / 2
		switch {
		case score < 0:
			return 0
		case score > 1:
			return 1
		default:
			return score
		}
	case DistanceCosine:
		fallthrough
	default:
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

func marshalMetadata(m metadata.Map) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalMetadata(b []byte) (metadata.Map, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out metadata.Map
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
