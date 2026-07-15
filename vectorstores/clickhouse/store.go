package clickhouse

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "ClickHouse"

const (
	DefaultTableName       = "vector_store"
	DefaultIDColumn        = "id"
	DefaultContentColumn   = "content"
	DefaultMetadataColumn  = "metadata"
	DefaultEmbeddingColumn = "embedding"
	DefaultDimensions      = 1536
	DefaultDistanceMetric  = DistanceCosine
)

// DistanceMetric selects the distance function ClickHouse uses to
// rank rows.
type DistanceMetric string

const (
	// DistanceCosine uses cosineDistance(a, b) — returns 1 - cosine
	// similarity, range [0, 2].
	DistanceCosine DistanceMetric = "cosine"

	// DistanceL2 uses L2Distance(a, b) — Euclidean distance,
	// range [0, ∞).
	DistanceL2 DistanceMetric = "l2"
)

// safeIdentifier matches the standard SQL unquoted identifier shape.

// StoreConfig contains configuration options for the ClickHouse
// vector store. The default schema uses `Map(String, String)` for
// metadata to keep the visitor's column-subscript syntax simple;
// callers needing typed metadata columns should manage the schema
// themselves and set InitializeSchema=false.
type StoreConfig struct {
	Context context.Context

	// Conn is the clickhouse-go v2 driver connection. Required.
	Conn driver.Conn

	// DatabaseName is the optional database prefix; empty uses the
	// connection's current database.
	DatabaseName string

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
	if c.Conn == nil {
		return errors.New("clickhouse: Conn is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("clickhouse: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("clickhouse: DocumentBatcher is required")
	}
	checks := map[string]string{
		"TableName":       c.TableName,
		"IDColumn":        c.IDColumn,
		"ContentColumn":   c.ContentColumn,
		"MetadataColumn":  c.MetadataColumn,
		"EmbeddingColumn": c.EmbeddingColumn,
	}
	if c.DatabaseName != "" {
		checks["DatabaseName"] = c.DatabaseName
	}
	return ident.Check("clickhouse", checks)
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

// Store is a ClickHouse-backed the vectorstore capability interfaces implementation.
type Store struct {
	conn            driver.Conn
	databaseName    string
	tableName       string
	fullTable       string
	idColumn        string
	contentColumn   string
	metadataColumn  string
	embeddingColumn string
	embeddingModel  embedding.Model
	embeddingClient *embeddingclient.Client
	documentBatcher vectorstores.Batcher
	dimensions      int
	distanceMetric  DistanceMetric
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: failed to create embedding client: %w", err)
	}
	fullTable := config.TableName
	if config.DatabaseName != "" {
		fullTable = config.DatabaseName + "." + config.TableName
	}
	store := &Store{
		conn:            config.Conn,
		databaseName:    config.DatabaseName,
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
		return nil, fmt.Errorf("clickhouse: failed to initialize store: %w", err)
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		dimensions, err := embedding.ResolveDimensions(ctx, s.embeddingModel)
		if err != nil {
			return fmt.Errorf("clickhouse: resolve embedding dimensions: %w", err)
		}
		s.dimensions = dimensions
	}
	if s.dimensions <= 0 {
		return errors.New("clickhouse: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}

	stmt := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			%s String,
			%s String,
			%s Map(String, String),
			%s Array(Float32),
			CONSTRAINT vec_len CHECK length(%s) = %d,
			INDEX vec_idx %s TYPE vector_similarity('hnsw', '%s', %d) GRANULARITY 1
		) ENGINE = MergeTree() ORDER BY (%s)`,
		s.fullTable,
		s.idColumn,
		s.contentColumn,
		s.metadataColumn,
		s.embeddingColumn,
		s.embeddingColumn, s.dimensions,
		s.embeddingColumn, indexDistance(s.distanceMetric), s.dimensions,
		s.idColumn,
	)
	if err := s.conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create table %s: %w", s.fullTable, err)
	}
	return nil
}

// indexDistance maps a DistanceMetric to ClickHouse's
// vector_similarity index distance parameter name.
func indexDistance(metric DistanceMetric) string {
	switch metric {
	case DistanceL2:
		return "L2Distance"
	case DistanceCosine:
		fallthrough
	default:
		return "cosineDistance"
	}
}

func distanceFunc(metric DistanceMetric) string {
	switch metric {
	case DistanceL2:
		return "L2Distance"
	case DistanceCosine:
		fallthrough
	default:
		return "cosineDistance"
	}
}

// Add embeds documents and inserts them as a single batch.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "clickhouse", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("clickhouse: failed to batch documents: %w", err)
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s, %s, %s, %s)",
		s.fullTable, s.idColumn, s.contentColumn, s.metadataColumn, s.embeddingColumn,
	)

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("clickhouse: failed to generate embeddings: %w", err)
		}

		batch, err := s.conn.PrepareBatch(ctx, insertSQL)
		if err != nil {
			return fmt.Errorf("clickhouse: prepare batch: %w", err)
		}

		appendErr := func() error {
			for i, doc := range docs {
				id := doc.ID
				if id == "" {
					id = uuid.NewString()
				}
				meta, err := metadataAsStringMap(doc.Metadata)
				if err != nil {
					return fmt.Errorf("metadata for %s: %w", id, err)
				}
				vec32 := math.ConvertSlice[float64, float32](vectors[i])
				if err := batch.Append(id, doc.Text, meta, vec32); err != nil {
					return fmt.Errorf("append %s: %w", id, err)
				}
			}
			return batch.Send()
		}()
		if appendErr != nil {
			return appendErr
		}
	}
	return nil
}

// Search runs an ANN search using the configured distance function.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("clickhouse: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "clickhouse", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	wherePredicate, whereArgs, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}
	wherePart := ""
	if wherePredicate != "" {
		wherePart = " AND " + wherePredicate
	}

	stmt := fmt.Sprintf(
		`SELECT %s, %s, %s, %s(%s, ?) AS distance FROM %s WHERE 1=1%s ORDER BY distance ASC LIMIT ?`,
		s.idColumn, s.contentColumn, s.metadataColumn,
		distanceFunc(s.distanceMetric), s.embeddingColumn,
		s.fullTable, wherePart,
	)

	args := make([]any, 0, len(whereArgs)+2)
	args = append(args, queryVec)
	args = append(args, whereArgs...)
	args = append(args, req.TopK)

	rows, err := s.conn.Query(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("clickhouse: query %s: %w", s.fullTable, err)
	}
	defer rows.Close()

	docs = make([]vectorstore.Match, 0, req.TopK)
	for rows.Next() {
		var (
			id       string
			content  string
			metaRaw  map[string]string
			distance float64
		)
		if err := rows.Scan(&id, &content, &metaRaw, &distance); err != nil {
			return nil, fmt.Errorf("clickhouse: scan row: %w", err)
		}
		score := distanceToScore(s.distanceMetric, distance)
		if score < req.MinScore {
			continue
		}
		metadata, err := stringMapToMetadata(metaRaw)
		if err != nil {
			return nil, fmt.Errorf("clickhouse: encode metadata: %w", err)
		}
		docs = append(docs, vectorstore.Match{
			Document: &document.Document{ID: id, Text: content, Metadata: metadata},
			Score:    score,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clickhouse: read rows: %w", err)
	}
	return docs, nil
}

// Delete removes rows matching the filter expression.
//
// ClickHouse mutations are asynchronous — callers should consider
// MutationOptions for synchronous behavior in their environment.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "clickhouse")
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
		return errors.New("clickhouse: refusing to delete on empty filter")
	}
	stmt := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s", s.fullTable, predicate)
	if err := s.conn.Exec(ctx, stmt, args...); err != nil {
		return fmt.Errorf("clickhouse: delete from %s: %w", s.fullTable, err)
	}
	return nil
}

// DeleteIDs removes rows by primary key via an `ALTER TABLE ...
// DELETE WHERE <id> IN (?, ...)` mutation, matching the form Delete
// uses. An empty slice is a no-op; unknown ids are silently ignored.
// Implements [vectorstore.IDDeleter].
//
// ClickHouse mutations are asynchronous — callers should consider
// MutationOptions for synchronous behavior in their environment.
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "clickhouse")
	defer func() { tracing.Finish(span, err) }()

	placeholders := strings.Repeat("?, ", len(ids)-1) + "?"
	stmt := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s IN (%s)", s.fullTable, s.idColumn, placeholders)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if err = s.conn.Exec(ctx, stmt, args...); err != nil {
		return fmt.Errorf("clickhouse: delete by ids from %s: %w", s.fullTable, err)
	}
	return nil
}

func (s *Store) buildFilter(filter filter.Predicate) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor(s.metadataColumn)
	if err := v.Visit(filter); err != nil {
		return "", nil, fmt.Errorf("clickhouse: convert filter: %w", err)
	}
	predicate, args := v.Result()
	return predicate, args, nil
}

func (s *Store) Close() error { return nil }

// distanceToScore maps a raw distance onto a [0, 1] similarity score.
func distanceToScore(metric DistanceMetric, distance float64) float64 {
	switch metric {
	case DistanceL2:
		return 1.0 / (1.0 + distance)
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

// metadataAsStringMap stringifies metadata values so they fit the
// `Map(String, String)` column. Complex values get JSON-encoded.
func metadataAsStringMap(m metadata.Map) (map[string]string, error) {
	values, err := m.Values()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		switch val := v.(type) {
		case string:
			out[k] = val
		case nil:
			out[k] = ""
		default:
			if b, err := json.Marshal(val); err == nil {
				out[k] = string(b)
			} else {
				out[k] = fmt.Sprint(val)
			}
		}
	}
	return out, nil
}

func stringMapToMetadata(m map[string]string) (metadata.Map, error) {
	if len(m) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return metadata.FromValues(out)
}
