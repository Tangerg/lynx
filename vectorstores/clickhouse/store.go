package clickhouse

import (
	"context"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
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
	DocumentBatcher document.Batcher

	Dimensions          int
	DistanceMetric      DistanceMetric
	InitializeSchema    bool
}

func (c StoreConfig) Validate() error {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Conn == nil {
		return errors.New("clickhouse: Conn is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("clickhouse: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("clickhouse: DocumentBatcher is required")
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
	if c.DatabaseName != "" {
		checks["DatabaseName"] = c.DatabaseName
	}
	return ident.Check("clickhouse", checks)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a ClickHouse-backed [vectorstore.Store] implementation.
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
	embeddingClient *embedding.Client
	documentBatcher document.Batcher
	dimensions      int
	distanceMetric  DistanceMetric
}


func NewStore(config StoreConfig) (*Store, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
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
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
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

// Create embeds documents and inserts them as a single batch.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("clickhouse: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "clickhouse", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("clickhouse: failed to batch documents: %w", err)
	}

	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s, %s, %s, %s)",
		s.fullTable, s.idColumn, s.contentColumn, s.metadataColumn, s.embeddingColumn,
	)

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
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
				meta := metadataAsStringMap(doc.Metadata)
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

// Retrieve runs an ANN search using the configured distance function.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("clickhouse: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "clickhouse", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
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

	docs = make([]*document.Document, 0, req.TopK)
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
		docs = append(docs, &document.Document{
			ID:       id,
			Text:     content,
			Score:    score,
			Metadata: stringMapToMetadata(metaRaw),
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
// MutationOptions for synchronous behaviour in their environment.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("clickhouse: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "clickhouse")
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
		return errors.New("clickhouse: refusing to delete on empty filter")
	}
	stmt := fmt.Sprintf("ALTER TABLE %s DELETE WHERE %s", s.fullTable, predicate)
	if err := s.conn.Exec(ctx, stmt, args...); err != nil {
		return fmt.Errorf("clickhouse: delete from %s: %w", s.fullTable, err)
	}
	return nil
}

func (s *Store) buildFilter(filter ast.Expr) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor(s.metadataColumn)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", nil, fmt.Errorf("clickhouse: convert filter: %w", err)
	}
	predicate, args := v.Result()
	return predicate, args, nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.conn,
		Provider:     Provider,
	}
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
func metadataAsStringMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
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
	return out
}

func stringMapToMetadata(m map[string]string) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
