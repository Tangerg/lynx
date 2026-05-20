package mariadb

import (
	"context"
	"cmp"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "MariaDB"

const (
	DefaultTableName       = "vector_store"
	DefaultIDColumn        = "id"
	DefaultContentColumn   = "content"
	DefaultMetadataColumn  = "metadata"
	DefaultEmbeddingColumn = "embedding"
	DefaultDimensions      = 1536
	DefaultDistanceMetric  = DistanceCosine
)

// DistanceMetric selects the vec_distance_<metric> function used at
// query time and the distance ordering MariaDB applies under the
// vector index.
type DistanceMetric string

const (
	// DistanceCosine — cosine distance. Default.
	DistanceCosine DistanceMetric = "cosine"

	// DistanceEuclidean — Euclidean (L2) distance.
	DistanceEuclidean DistanceMetric = "euclidean"
)

// safeIdentifier matches the standard SQL unquoted identifier shape.

// StoreConfig contains configuration options for the MariaDB vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// DB is the database handle. Required. Use a *sql.DB built from
	// the github.com/go-sql-driver/mysql driver pointed at a MariaDB
	// 11.7+ instance with vector support enabled.
	DB *sql.DB

	// SchemaName is the optional schema (database) prefix. When
	// empty the connection's default database is used.
	SchemaName string

	// TableName is the table that stores documents and their
	// embeddings. Optional: defaults to [DefaultTableName].
	TableName string

	// IDColumn / ContentColumn / MetadataColumn / EmbeddingColumn
	// override the column names of the generated schema. Each
	// defaults to its respective Default* constant when empty.
	IDColumn        string
	ContentColumn   string
	MetadataColumn  string
	EmbeddingColumn string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before insertion. Required.
	DocumentBatcher document.Batcher

	// Dimensions sets the VECTOR column width. When zero, falls
	// back to the embedding model's reported value and then
	// [DefaultDimensions].
	Dimensions int

	// DistanceMetric selects the distance function. Optional:
	// defaults to [DistanceCosine].
	DistanceMetric DistanceMetric

	// InitializeSchema, when true, creates the table + vector index
	// if they don't already exist.
	InitializeSchema bool
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("mariadb: config must not be nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.DB == nil {
		return errors.New("mariadb: DB is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("mariadb: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("mariadb: DocumentBatcher is required")
	}

	c.TableName = cmp.Or(c.TableName, DefaultTableName)
	c.IDColumn = cmp.Or(c.IDColumn, DefaultIDColumn)
	c.ContentColumn = cmp.Or(c.ContentColumn, DefaultContentColumn)
	c.MetadataColumn = cmp.Or(c.MetadataColumn, DefaultMetadataColumn)
	c.EmbeddingColumn = cmp.Or(c.EmbeddingColumn, DefaultEmbeddingColumn)
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DefaultDistanceMetric)

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
	return ident.Check("mariadb", checks)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a MariaDB-backed [vectorstore.Store] implementation using
// the VECTOR column type and vec_distance_* functions introduced in
// MariaDB 11.6+.
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
	documentBatcher document.Batcher
	dimensions      int
	distanceMetric  DistanceMetric
}


func NewStore(config *StoreConfig) (*Store, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("mariadb: failed to create embedding client: %w", err)
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
		return nil, fmt.Errorf("mariadb: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensionality and provisions the table when
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
		return errors.New("mariadb: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}
	if s.schemaName != "" {
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", s.schemaName)); err != nil {
			return fmt.Errorf("create schema %s: %w", s.schemaName, err)
		}
	}

	stmt := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			%s VARCHAR(64) NOT NULL PRIMARY KEY,
			%s TEXT,
			%s JSON,
			%s VECTOR(%d) NOT NULL,
			VECTOR INDEX %s_idx (%s)
		) ENGINE=InnoDB`,
		s.fullTable,
		s.idColumn,
		s.contentColumn,
		s.metadataColumn,
		s.embeddingColumn, s.dimensions,
		s.tableName, s.embeddingColumn,
	)
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("create table %s: %w", s.fullTable, err)
	}
	return nil
}

// Create embeds documents and upserts them into the vector table.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("mariadb: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "mariadb", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("mariadb: failed to batch documents: %w", err)
	}

	upsert := fmt.Sprintf(
		`INSERT INTO %s (%s, %s, %s, %s) VALUES (?, ?, ?, VEC_FromText(?)) `+
			`ON DUPLICATE KEY UPDATE %s = VALUES(%s), %s = VALUES(%s), %s = VALUES(%s)`,
		s.fullTable, s.idColumn, s.contentColumn, s.metadataColumn, s.embeddingColumn,
		s.contentColumn, s.contentColumn,
		s.metadataColumn, s.metadataColumn,
		s.embeddingColumn, s.embeddingColumn,
	)

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("mariadb: failed to generate embeddings: %w", err)
		}

		stmt, err := s.db.PrepareContext(ctx, upsert)
		if err != nil {
			return fmt.Errorf("mariadb: prepare upsert: %w", err)
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

// Retrieve embeds the query, ranks rows by vec_distance, and returns
// matching documents above MinScore.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("mariadb: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "mariadb", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("mariadb: failed to embed query: %w", err)
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

	stmt := fmt.Sprintf(
		`SELECT %s, %s, %s, vec_distance_%s(%s, VEC_FromText(?)) AS distance `+
			`FROM %s WHERE 1=1%s ORDER BY distance ASC LIMIT ?`,
		s.idColumn, s.contentColumn, s.metadataColumn,
		s.distanceMetric, s.embeddingColumn,
		s.fullTable, wherePart,
	)

	args := make([]any, 0, len(whereArgs)+2)
	args = append(args, vecText)
	args = append(args, whereArgs...)
	args = append(args, req.TopK)

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("mariadb: query %s: %w", s.fullTable, err)
	}
	defer rows.Close()

	docs = make([]*document.Document, 0, req.TopK)
	for rows.Next() {
		var (
			id       string
			content  sql.NullString
			metaRaw  sql.NullString
			distance float64
		)
		if err := rows.Scan(&id, &content, &metaRaw, &distance); err != nil {
			return nil, fmt.Errorf("mariadb: scan row: %w", err)
		}

		score := distanceToScore(s.distanceMetric, distance)
		if score < req.MinScore {
			continue
		}

		doc := &document.Document{ID: id, Score: score}
		if content.Valid {
			doc.Text = content.String
		}
		if metaRaw.Valid {
			if doc.Metadata, err = unmarshalMetadata([]byte(metaRaw.String)); err != nil {
				return nil, fmt.Errorf("mariadb: unmarshal metadata for %s: %w", id, err)
			}
		}
		docs = append(docs, doc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mariadb: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes rows matching the filter expression.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("mariadb: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "mariadb")
	defer func() { tracing.Finish(span, err) }()

	var (
		predicate string
		args      []any
	)
	predicate, args, err = s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("mariadb: refusing to delete on empty filter")
	}

	stmt := fmt.Sprintf("DELETE FROM %s WHERE %s", s.fullTable, predicate)
	if _, err := s.db.ExecContext(ctx, stmt, args...); err != nil {
		return fmt.Errorf("mariadb: delete from %s: %w", s.fullTable, err)
	}
	return nil
}

// buildFilter wraps the visitor.
func (s *Store) buildFilter(filter ast.Expr) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor(s.metadataColumn)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", nil, fmt.Errorf("mariadb: convert filter: %w", err)
	}
	predicate, args := v.Result()
	return predicate, args, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.db,
		Provider:     Provider,
	}
}


func (s *Store) Close() error { return nil }

// distanceToScore maps a vec_distance_* result into a [0, 1]
// similarity score.
func distanceToScore(metric DistanceMetric, distance float64) float64 {
	switch metric {
	case DistanceEuclidean:
		return 1.0 / (1.0 + distance)
	case DistanceCosine:
		fallthrough
	default:
		// MariaDB cosine distance ∈ [0, 2]; (1 - d/2) collapses to
		// [0, 1].
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


func marshalMetadata(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func unmarshalMetadata(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
