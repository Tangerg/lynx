package cassandra

import (
	"context"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gocql/gocql"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
)

const Provider = "Cassandra"

const (
	DefaultKeyspaceName    = "lynx"
	DefaultTableName       = "vector_store"
	DefaultIDColumn        = "id"
	DefaultContentColumn   = "content"
	DefaultMetadataColumn  = "metadata"
	DefaultEmbeddingColumn = "embedding"
	DefaultDimensions      = 1536
	DefaultSimilarity      = SimilarityCosine
)

// SimilarityFunction picks the function name used by the
// similarity_<func> built-in. The chosen value is recorded in the
// SAI index definition at creation time.
type SimilarityFunction string

const (
	// SimilarityCosine — cosine similarity. Default.
	SimilarityCosine SimilarityFunction = "cosine"

	// SimilarityDotProduct — dot product.
	SimilarityDotProduct SimilarityFunction = "dot_product"

	// SimilarityEuclidean — Euclidean (L2) distance, mapped to a
	// similarity score by Cassandra itself.
	SimilarityEuclidean SimilarityFunction = "euclidean"
)


// MetadataColumn declares a custom metadata column that the store
// indexes for filtering. Cassandra has no JSON-path operator, so each
// filterable metadata key must be a typed column on the table.
type MetadataColumn struct {
	// Name is the column identifier on the underlying table.
	Name string

	// CQLType is the column data type as written in CREATE TABLE
	// (e.g. "text", "int", "boolean", "double").
	CQLType string
}

// StoreConfig contains configuration options for the Cassandra vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// Session is the gocql session. Required.
	Session *gocql.Session

	// KeyspaceName is the keyspace that holds the vector table.
	// Optional: defaults to [DefaultKeyspaceName].
	KeyspaceName string

	// TableName is the table that stores documents and their
	// embeddings. Optional: defaults to [DefaultTableName].
	TableName string

	// IDColumn / ContentColumn / EmbeddingColumn / MetadataColumn —
	// override the column names of the generated schema. Each
	// defaults to its respective Default* constant when empty.
	IDColumn        string
	ContentColumn   string
	EmbeddingColumn string

	// MetadataColumns enumerates the filterable metadata keys. Each
	// becomes a typed column on the table and (under
	// InitializeSchema) an SAI index. The optional [DocumentMetadata]
	// helpers may populate these from the Document.Metadata map.
	MetadataColumns []MetadataColumn

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before insertion. Required.
	DocumentBatcher document.Batcher

	// Dimensions sets the VECTOR column width. When zero, falls
	// back to the embedding model's reported value and then
	// [DefaultDimensions].
	Dimensions int

	// Similarity selects the vector similarity function. Optional:
	// defaults to [SimilarityCosine].
	Similarity SimilarityFunction

	// InitializeSchema, when true, creates the keyspace, table, and
	// SAI vector index if they don't already exist.
	InitializeSchema bool

	// KeyspaceReplication is the replication clause used when
	// InitializeSchema creates the keyspace — e.g.
	// "{'class': 'SimpleStrategy', 'replication_factor': 1}".
	// Optional: defaults to a single-replica SimpleStrategy.
	KeyspaceReplication string
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("cassandra: config must not be nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Session == nil {
		return errors.New("cassandra: Session is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("cassandra: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("cassandra: DocumentBatcher is required")
	}

	c.KeyspaceName = cmp.Or(c.KeyspaceName, DefaultKeyspaceName)
	c.TableName = cmp.Or(c.TableName, DefaultTableName)
	c.IDColumn = cmp.Or(c.IDColumn, DefaultIDColumn)
	c.ContentColumn = cmp.Or(c.ContentColumn, DefaultContentColumn)
	c.EmbeddingColumn = cmp.Or(c.EmbeddingColumn, DefaultEmbeddingColumn)
	c.Similarity = cmp.Or(c.Similarity, DefaultSimilarity)
	if c.KeyspaceReplication == "" {
		c.KeyspaceReplication = "{'class': 'SimpleStrategy', 'replication_factor': 1}"
	}

	checks := map[string]string{
		"KeyspaceName":    c.KeyspaceName,
		"TableName":       c.TableName,
		"IDColumn":        c.IDColumn,
		"ContentColumn":   c.ContentColumn,
		"EmbeddingColumn": c.EmbeddingColumn,
	}
	for _, m := range c.MetadataColumns {
		if m.Name == "" {
			return errors.New("cassandra: MetadataColumn.Name must not be empty")
		}
		checks["MetadataColumn."+m.Name] = m.Name
		if m.CQLType == "" {
			return fmt.Errorf("cassandra: MetadataColumn %q must have a CQLType", m.Name)
		}
	}
	return ident.Check("cassandra", checks)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Cassandra 5.0+ backed [vectorstore.Store] implementation.
// It relies on the VECTOR column type and SAI indexes.
type Store struct {
	session         *gocql.Session
	keyspaceName    string
	tableName       string
	fullTable       string
	idColumn        string
	contentColumn   string
	embeddingColumn string
	metadataColumns []MetadataColumn
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher document.Batcher
	dimensions      int
	similarity      SimilarityFunction
}


func NewStore(config *StoreConfig) (*Store, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("cassandra: failed to create embedding client: %w", err)
	}

	store := &Store{
		session:         config.Session,
		keyspaceName:    config.KeyspaceName,
		tableName:       config.TableName,
		fullTable:       config.KeyspaceName + "." + config.TableName,
		idColumn:        config.IDColumn,
		contentColumn:   config.ContentColumn,
		embeddingColumn: config.EmbeddingColumn,
		metadataColumns: config.MetadataColumns,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
		similarity:      config.Similarity,
	}

	if err = store.initialize(config.Context, config.InitializeSchema, config.KeyspaceReplication); err != nil {
		return nil, fmt.Errorf("cassandra: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensions and provisions the schema when
// requested.
func (s *Store) initialize(ctx context.Context, initSchema bool, replication string) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("cassandra: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}

	stmts := []string{
		fmt.Sprintf("CREATE KEYSPACE IF NOT EXISTS %s WITH REPLICATION = %s",
			s.keyspaceName, replication),
	}

	// Build CREATE TABLE with all declared metadata columns.
	var cols strings.Builder
	cols.WriteString(s.idColumn)
	cols.WriteString(" text PRIMARY KEY, ")
	cols.WriteString(s.contentColumn)
	cols.WriteString(" text, ")
	cols.WriteString(s.embeddingColumn)
	cols.WriteString(fmt.Sprintf(" vector<float, %d>", s.dimensions))
	for _, m := range s.metadataColumns {
		cols.WriteString(", ")
		cols.WriteString(m.Name)
		cols.WriteString(" ")
		cols.WriteString(m.CQLType)
	}
	stmts = append(stmts, fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (%s)",
		s.fullTable, cols.String(),
	))

	// Vector SAI index for ANN search.
	stmts = append(stmts, fmt.Sprintf(
		"CREATE CUSTOM INDEX IF NOT EXISTS %s_vec_idx ON %s (%s) USING 'StorageAttachedIndex' "+
			"WITH OPTIONS = {'similarity_function': '%s'}",
		s.tableName, s.fullTable, s.embeddingColumn, s.similarity,
	))

	// SAI index per metadata column so the visitor's WHERE
	// predicates can run without ALLOW FILTERING.
	for _, m := range s.metadataColumns {
		stmts = append(stmts, fmt.Sprintf(
			"CREATE CUSTOM INDEX IF NOT EXISTS %s_%s_idx ON %s (%s) USING 'StorageAttachedIndex'",
			s.tableName, m.Name, s.fullTable, m.Name,
		))
	}

	for _, stmt := range stmts {
		if err := s.session.Query(stmt).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("execute %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// Create embeds documents and inserts them.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("cassandra: invalid create request: %w", err)
	}

	batchedDocs, err := s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("cassandra: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("cassandra: failed to generate embeddings: %w", err)
		}

		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			if err := s.insertOne(ctx, id, doc, vectors[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// insertOne issues an UPSERT (INSERT in Cassandra always upserts on
// primary key). The vector is inlined as a CQL literal because the
// gocql v1.x driver doesn't support typed vector binding.
func (s *Store) insertOne(ctx context.Context, id string, doc *document.Document, vec []float64) error {
	columns := []string{s.idColumn, s.contentColumn, s.embeddingColumn}
	placeholders := []string{"?", "?", docio.FormatVectorLiteral(math.ConvertSlice[float64, float32](vec))}
	args := []any{id, doc.Text}

	for _, m := range s.metadataColumns {
		if val, ok := doc.Metadata[m.Name]; ok {
			columns = append(columns, m.Name)
			placeholders = append(placeholders, "?")
			args = append(args, val)
		}
	}

	stmt := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		s.fullTable, strings.Join(columns, ", "), strings.Join(placeholders, ", "),
	)
	if err := s.session.Query(stmt, args...).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("cassandra: insert %s: %w", id, err)
	}
	return nil
}

// Retrieve runs an ANN query using the configured similarity function.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("cassandra: invalid retrieval request: %w", err)
	}

	vector, _, err := s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("cassandra: failed to embed query: %w", err)
	}
	vecLiteral := docio.FormatVectorLiteral(math.ConvertSlice[float64, float32](vector))

	wherePredicate, whereArgs, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	wherePart := ""
	if wherePredicate != "" {
		wherePart = " WHERE " + wherePredicate
	}

	columns := []string{
		s.idColumn,
		s.contentColumn,
		fmt.Sprintf("similarity_%s(%s, %s) AS score", s.similarity, s.embeddingColumn, vecLiteral),
	}
	for _, m := range s.metadataColumns {
		columns = append(columns, m.Name)
	}

	stmt := fmt.Sprintf(
		"SELECT %s FROM %s%s ORDER BY %s ANN OF %s LIMIT %d",
		strings.Join(columns, ", "), s.fullTable, wherePart,
		s.embeddingColumn, vecLiteral, req.TopK,
	)

	iter := s.session.Query(stmt, whereArgs...).WithContext(ctx).Iter()
	defer iter.Close()

	docs := make([]*document.Document, 0, req.TopK)
	scanDest := s.makeScanDestinations()
	for iter.Scan(scanDest...) {
		doc, err := s.scanDestToDocument(scanDest, req.MinScore)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		docs = append(docs, doc)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("cassandra: query %s: %w", s.fullTable, err)
	}
	return docs, nil
}

// makeScanDestinations allocates the per-row pointer slice used by
// gocql.Iter.Scan. The shape mirrors the SELECT column list built in
// Retrieve.
func (s *Store) makeScanDestinations() []any {
	dest := []any{new(string), new(string), new(float32)}
	for range s.metadataColumns {
		dest = append(dest, new(any))
	}
	return dest
}

// scanDestToDocument turns the per-row pointer slice back into a
// Document. Returns nil when the row's score falls below minScore.
func (s *Store) scanDestToDocument(dest []any, minScore float64) (*document.Document, error) {
	id := *dest[0].(*string)
	text := *dest[1].(*string)
	score := float64(*dest[2].(*float32))
	if score < minScore {
		return nil, nil
	}

	doc := &document.Document{ID: id, Text: text, Score: score}
	if len(s.metadataColumns) > 0 {
		meta := make(map[string]any, len(s.metadataColumns))
		for i, m := range s.metadataColumns {
			v := *(dest[3+i].(*any))
			if v != nil {
				meta[m.Name] = v
			}
		}
		if len(meta) > 0 {
			doc.Metadata = meta
		}
	}
	return doc, nil
}

// Delete removes rows matching the filter expression.
//
// Cassandra doesn't allow filter-based DELETE without a primary-key
// equality clause; the SAI path supports it only via secondary
// indexes. To stay portable we look up matching primary keys first,
// then issue per-row DELETEs.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("cassandra: invalid delete request: %w", err)
	}

	predicate, args, err := s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("cassandra: refusing to delete on empty filter")
	}

	selectStmt := fmt.Sprintf("SELECT %s FROM %s WHERE %s", s.idColumn, s.fullTable, predicate)
	iter := s.session.Query(selectStmt, args...).WithContext(ctx).Iter()
	defer iter.Close()

	var ids []string
	var id string
	for iter.Scan(&id) {
		ids = append(ids, id)
	}
	if err := iter.Close(); err != nil {
		return fmt.Errorf("cassandra: enumerate ids: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}

	deleteStmt := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", s.fullTable, s.idColumn)
	for _, id := range ids {
		if err := s.session.Query(deleteStmt, id).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("cassandra: delete %s: %w", id, err)
		}
	}
	return nil
}

func (s *Store) buildFilter(filter ast.Expr) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor()
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", nil, fmt.Errorf("cassandra: convert filter: %w", err)
	}
	predicate, args := v.Result()
	return predicate, args, nil
}

func (s *Store) Info() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: s.session,
		Provider:     Provider,
	}
}


func (s *Store) Close() error { return nil }


// marshalMetadata / unmarshalMetadata are unused right now but kept
// for future compatibility with stores that switch metadata storage
// from columns to a JSON blob.
var (
	_ = json.Marshal
	_ = json.Unmarshal
)
