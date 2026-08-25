package cockroachdb

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/vectorstores/internal/identifier"
	"github.com/Tangerg/lynx/vectorstores/postgres/internal/pgstore"
)

const Provider = "CockroachDB"

const (
	DefaultSchemaName     = "public"
	DefaultTableName      = "vector_store"
	DefaultMetadataColumn = "metadata"
	DefaultIndexSuffix    = "_embedding_idx"
)

// DistanceMetric selects both the pgvector-compatible query operator and the
// native CockroachDB vector-index opclass.
type DistanceMetric string

const (
	DistanceCosine DistanceMetric = "cosine"
	DistanceL2     DistanceMetric = "l2"
	DistanceIP     DistanceMetric = "ip"
)

func (metric DistanceMetric) indexOpClass() string {
	switch metric {
	case DistanceL2:
		return "vector_l2_ops"
	case DistanceIP:
		return "vector_ip_ops"
	default:
		return "vector_cosine_ops"
	}
}

// StoreConfig configures a native CockroachDB vector store.
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
	InitializeSchema bool
}

func (c *StoreConfig) applyDefaults() {
	c.SchemaName = cmp.Or(c.SchemaName, DefaultSchemaName)
	c.TableName = cmp.Or(c.TableName, DefaultTableName)
	c.MetadataColumn = cmp.Or(c.MetadataColumn, DefaultMetadataColumn)
	if c.IndexName == "" {
		c.IndexName = c.TableName + DefaultIndexSuffix
	}
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DistanceCosine)
}

func (c StoreConfig) Validate() error {
	c.applyDefaults()
	if c.Pool == nil {
		return errors.New("cockroachdb: Pool is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("cockroachdb: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("cockroachdb: DocumentBatcher is required")
	}
	if c.Dimensions < 0 {
		return errors.New("cockroachdb: Dimensions must be >= 0")
	}
	switch c.DistanceMetric {
	case DistanceCosine, DistanceL2, DistanceIP:
	default:
		return fmt.Errorf("cockroachdb: unsupported DistanceMetric %q", c.DistanceMetric)
	}
	return identifier.Strict.Validate("cockroachdb", map[string]string{
		"SchemaName":     c.SchemaName,
		"TableName":      c.TableName,
		"IndexName":      c.IndexName,
		"MetadataColumn": c.MetadataColumn,
	})
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store implements vector-store capabilities with CockroachDB's native VECTOR
// type and vector indexes.
type Store struct {
	engine *pgstore.Store
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if err := initialize(ctx, config); err != nil {
		return nil, fmt.Errorf("cockroachdb.NewStore: initialize schema: %w", err)
	}

	engine, err := pgstore.New(pgstore.Config{
		Provider:        "cockroachdb",
		Pool:            config.Pool,
		SchemaName:      config.SchemaName,
		TableName:       config.TableName,
		MetadataColumn:  config.MetadataColumn,
		EmbeddingModel:  config.EmbeddingModel,
		DocumentBatcher: config.DocumentBatcher,
		DistanceMetric:  pgstore.DistanceMetric(config.DistanceMetric),
	})
	if err != nil {
		return nil, fmt.Errorf("cockroachdb.NewStore: %w", err)
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

	if _, err := config.Pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", config.SchemaName)); err != nil {
		return fmt.Errorf("create schema %s: %w", config.SchemaName, err)
	}
	fullTable := config.SchemaName + "." + config.TableName
	statement := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id STRING PRIMARY KEY,
		content STRING,
		%s JSONB,
		embedding VECTOR(%d),
		VECTOR INDEX %s (embedding %s)
	)`, fullTable, config.MetadataColumn, dimensions, config.IndexName, config.DistanceMetric.indexOpClass())
	if _, err := config.Pool.Exec(ctx, statement); err != nil {
		return fmt.Errorf("create table %s: %w", fullTable, err)
	}
	return nil
}

func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("cockroachdb.Store.Index: %w", err)
	}
	return s.engine.Index(ctx, request)
}

func (s *Store) Search(ctx context.Context, request *vectorstore.SearchRequest) (*vectorstore.SearchResponse, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("cockroachdb.Store.Search: %w", err)
	}
	return s.engine.Search(ctx, request)
}

func (s *Store) DeleteWhere(ctx context.Context, predicate filter.Predicate) error {
	if predicate == nil {
		return vectorstore.ErrMissingFilter
	}
	if err := predicate.Validate(); err != nil {
		return fmt.Errorf("cockroachdb.Store.DeleteWhere: %w", err)
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
