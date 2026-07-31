package pgvector

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/pgstore"
)

const Provider = "PgVector"

const (
	DefaultSchemaName     = "public"
	DefaultTableName      = "vector_store"
	DefaultMetadataColumn = "metadata"
	DefaultIndexSuffix    = "_embedding_idx"
)

// DistanceMetric selects the query operator and ANN-index opclass.
type DistanceMetric string

const (
	// DistanceCosine uses cosine distance (`<=>` and vector_cosine_ops).
	DistanceCosine DistanceMetric = "cosine"

	// DistanceL2 uses Euclidean distance (`<->` and vector_l2_ops).
	DistanceL2 DistanceMetric = "l2"

	// DistanceIP uses negative inner product (`<#>` and vector_ip_ops).
	DistanceIP DistanceMetric = "ip"
)

// IndexType selects the ANN index created during schema initialization.
type IndexType string

const (
	IndexHNSW    IndexType = "hnsw"
	IndexIVFFlat IndexType = "ivfflat"
	IndexNone    IndexType = "none"
)

// StoreConfig configures a pgvector-backed store.
type StoreConfig struct {
	Pool             *pgxpool.Pool
	SchemaName       string
	TableName        string
	IndexName        string
	MetadataColumn   string
	EmbeddingModel   embedding.Model
	DocumentBatcher  vectorstores.Batcher
	Dimensions       int
	DistanceMetric   DistanceMetric
	IndexType        IndexType
	InitializeSchema bool
}

func (c StoreConfig) Validate() error {
	c.applyDefaults()
	if c.Pool == nil {
		return errors.New("pgvector: Pool is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("pgvector: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("pgvector: DocumentBatcher is required")
	}
	if c.Dimensions < 0 {
		return errors.New("pgvector: Dimensions must be >= 0")
	}
	switch c.DistanceMetric {
	case DistanceCosine, DistanceL2, DistanceIP:
	default:
		return fmt.Errorf("pgvector: unsupported DistanceMetric %q", c.DistanceMetric)
	}
	switch c.IndexType {
	case IndexHNSW, IndexIVFFlat, IndexNone:
	default:
		return fmt.Errorf("pgvector: unsupported IndexType %q", c.IndexType)
	}
	return ident.Check("pgvector", map[string]string{
		"SchemaName":     c.SchemaName,
		"TableName":      c.TableName,
		"IndexName":      c.IndexName,
		"MetadataColumn": c.MetadataColumn,
	})
}

func (c *StoreConfig) applyDefaults() {
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
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store implements vector-store capabilities with PostgreSQL and pgvector.
type Store struct {
	engine *pgstore.Store
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := initialize(ctx, config); err != nil {
		return nil, fmt.Errorf("pgvector.NewStore: initialize schema: %w", err)
	}

	engine, err := pgstore.New(pgstore.Config{
		Provider:        "pgvector",
		Pool:            config.Pool,
		SchemaName:      config.SchemaName,
		TableName:       config.TableName,
		MetadataColumn:  config.MetadataColumn,
		EmbeddingModel:  config.EmbeddingModel,
		DocumentBatcher: config.DocumentBatcher,
		DistanceMetric:  pgstore.DistanceMetric(config.DistanceMetric),
	})
	if err != nil {
		return nil, fmt.Errorf("pgvector.NewStore: %w", err)
	}
	return &Store{engine: engine}, nil
}

func initialize(ctx context.Context, config StoreConfig) error {
	if !config.InitializeSchema {
		return nil
	}

	dimensions := config.Dimensions
	if dimensions <= 0 {
		client, err := embeddingclient.New(config.EmbeddingModel)
		if err != nil {
			return fmt.Errorf("create embedding client: %w", err)
		}
		dimensions, err = client.Dimensions(ctx)
		if err != nil {
			return fmt.Errorf("resolve embedding dimensions: %w", err)
		}
	}
	if dimensions <= 0 {
		return errors.New("Dimensions must be > 0")
	}

	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, config.SchemaName),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.%s (
			id text PRIMARY KEY,
			content text,
			%s jsonb,
			embedding vector(%d)
		)`, config.SchemaName, config.TableName, config.MetadataColumn, dimensions),
	}
	if config.IndexType != IndexNone {
		statements = append(statements, fmt.Sprintf(
			`CREATE INDEX IF NOT EXISTS %s ON %s.%s USING %s (embedding %s)`,
			config.IndexName, config.SchemaName, config.TableName, config.IndexType,
			config.DistanceMetric.indexOpClass(),
		))
	}

	for _, statement := range statements {
		if _, err := config.Pool.Exec(ctx, statement); err != nil {
			return fmt.Errorf("execute %q: %w", strings.SplitN(statement, "\n", 2)[0], err)
		}
	}
	return nil
}

func (m DistanceMetric) indexOpClass() string {
	switch m {
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

func (s *Store) Add(ctx context.Context, docs []*document.Document) error {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("pgvector.Store.Add: %w", err)
	}
	return s.engine.Add(ctx, docs)
}

func (s *Store) Search(ctx context.Context, request vectorstore.SearchRequest) ([]vectorstore.Match, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector.Store.Search: %w", err)
	}
	return s.engine.Search(ctx, request)
}

func (s *Store) DeleteWhere(ctx context.Context, predicate filter.Predicate) error {
	if predicate == nil {
		return vectorstore.ErrMissingFilter
	}
	if err := filter.Validate(predicate); err != nil {
		return fmt.Errorf("pgvector.Store.DeleteWhere: %w", err)
	}
	return s.engine.DeleteWhere(ctx, predicate)
}

func (s *Store) DeleteIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.engine.DeleteIDs(ctx, ids)
}

func (s *Store) Close() error { return s.engine.Close() }
