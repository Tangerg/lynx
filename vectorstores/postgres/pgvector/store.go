package pgvector

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/postgres/internal/pgstore"
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

// Valid reports whether d is supported by pgvector.
func (d DistanceMetric) Valid() bool {
	switch d {
	case DistanceCosine, DistanceL2, DistanceIP:
		return true
	default:
		return false
	}
}

// String returns the pgvector distance token.
func (d DistanceMetric) String() string { return string(d) }

// IndexType selects the ANN index created during schema initialization.
type IndexType string

const (
	IndexHNSW    IndexType = "hnsw"
	IndexIVFFlat IndexType = "ivfflat"
	IndexNone    IndexType = "none"
)

// Valid reports whether i is supported by pgvector schema initialization.
func (i IndexType) Valid() bool {
	switch i {
	case IndexHNSW, IndexIVFFlat, IndexNone:
		return true
	default:
		return false
	}
}

// String returns the pgvector index token.
func (i IndexType) String() string { return string(i) }

// StoreConfig configures a pgvector-backed store.
type StoreConfig struct {
	Pool             *pgxpool.Pool
	SchemaName       string
	TableName        string
	IndexName        string
	MetadataColumn   string
	EmbeddingModel   embedding.Model
	DocumentBatcher  vectorstore.Batcher
	Dimensions       int
	DistanceMetric   DistanceMetric
	IndexType        IndexType
	InitializeSchema bool
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Pool == nil {
		return errors.New("pgvector: Pool is required")
	}
	if s.EmbeddingModel == nil {
		return errors.New("pgvector: EmbeddingModel is required")
	}
	if s.DocumentBatcher == nil {
		return errors.New("pgvector: DocumentBatcher is required")
	}
	if s.Dimensions < 0 {
		return errors.New("pgvector: Dimensions must be >= 0")
	}
	if !s.DistanceMetric.Valid() {
		return fmt.Errorf("pgvector: unsupported DistanceMetric %q", s.DistanceMetric)
	}
	if !s.IndexType.Valid() {
		return fmt.Errorf("pgvector: unsupported IndexType %q", s.IndexType)
	}
	return s.validateIdentifiers()
}

func (s StoreConfig) validateIdentifiers() error {
	if err := identifier(s.SchemaName).validate("SchemaName"); err != nil {
		return err
	}
	if err := identifier(s.TableName).validate("TableName"); err != nil {
		return err
	}
	if err := identifier(s.IndexName).validate("IndexName"); err != nil {
		return err
	}
	return identifier(s.MetadataColumn).validate("MetadataColumn")
}

func (s *StoreConfig) applyDefaults() {
	s.SchemaName = cmp.Or(s.SchemaName, DefaultSchemaName)
	s.TableName = cmp.Or(s.TableName, DefaultTableName)
	s.MetadataColumn = cmp.Or(s.MetadataColumn, DefaultMetadataColumn)
	if s.IndexName == "" {
		s.IndexName = s.TableName + DefaultIndexSuffix
	}
	s.DistanceMetric = cmp.Or(s.DistanceMetric, DistanceCosine)
	s.IndexType = cmp.Or(s.IndexType, IndexHNSW)
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
		return errors.New("dimensions must be > 0")
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

func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("pgvector.Store.Index: %w", err)
	}
	return s.engine.Index(ctx, request)
}

func (s *Store) Search(ctx context.Context, request *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("pgvector.Store.Search: %w", err)
	}
	return s.engine.Search(ctx, request)
}

func (s *Store) DeleteWhere(ctx context.Context, predicate filter.Predicate) error {
	if predicate == nil {
		return vectorstore.ErrMissingFilter
	}
	if err := predicate.Validate(); err != nil {
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
