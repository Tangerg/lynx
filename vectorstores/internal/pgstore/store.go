// Package pgstore implements the shared pgwire execution semantics used by
// PostgreSQL/pgvector and CockroachDB. Provider packages remain responsible for
// configuration validation and schema provisioning.
package pgstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvec "github.com/pgvector/pgvector-go"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/vectorstores/internal/batching"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/pgfilter"
	"github.com/Tangerg/lynx/vectorstores/internal/scores"
	vectorconv "github.com/Tangerg/lynx/vectorstores/internal/vector"
)

// DistanceMetric selects the pgvector-compatible query operator.
type DistanceMetric string

const (
	DistanceCosine DistanceMetric = "cosine"
	DistanceL2     DistanceMetric = "l2"
	DistanceIP     DistanceMetric = "ip"
)

// Config contains only shared execution dependencies. Schema policy belongs to
// the provider package and must be resolved before constructing the engine.
type Config struct {
	Provider        string
	Pool            *pgxpool.Pool
	SchemaName      string
	TableName       string
	MetadataColumn  string
	EmbeddingModel  embedding.Model
	DocumentBatcher vectorstore.Batcher
	DistanceMetric  DistanceMetric
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store executes pgvector-compatible data operations for one named provider.
type Store struct {
	provider        string
	pool            *pgxpool.Pool
	metadataColumn  string
	fullTable       string
	embeddingClient *embeddingclient.Client
	documentBatcher vectorstore.Batcher
	distanceMetric  DistanceMetric
}

// New constructs an execution engine after the provider has validated config
// and provisioned any requested schema.
func New(config Config) (*Store, error) {
	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("%s: create embedding client: %w", config.Provider, err)
	}

	return &Store{
		provider:        config.Provider,
		pool:            config.Pool,
		metadataColumn:  config.MetadataColumn,
		fullTable:       config.SchemaName + "." + config.TableName,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		distanceMetric:  config.DistanceMetric,
	}, nil
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
		return scores.Distance(distance)
	case DistanceIP:
		return scores.NegativeInnerProductDistance(distance)
	case DistanceCosine:
		fallthrough
	default:
		return scores.CosineDistance(distance)
	}
}

// Add embeds the documents and upserts them into the configured table.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("%s.Store.Add: %w", s.provider, err)
	}

	var batchedDocs [][]*document.Document
	batchedDocs, err = batching.Batch(ctx, s.documentBatcher, docs)
	if err != nil {
		return fmt.Errorf("%s.Store.Add: batch documents: %w", s.provider, err)
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
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("%s.Store.Add: embed documents: %w", s.provider, err)
		}

		batch := &pgx.Batch{}
		for i, doc := range docs {
			id := doc.ID

			metaJSON, err := marshalMetadata(doc.Metadata)
			if err != nil {
				return fmt.Errorf("%s.Store.Add: marshal metadata for document %q: %w", s.provider, id, err)
			}

			vec := pgvec.NewVector(vectorconv.Float32(vectors[i]))
			batch.Queue(upsertSQL, id, doc.Text, metaJSON, vec)
		}

		results := s.pool.SendBatch(ctx, batch)
		execErr := drainBatch(results, len(docs))
		closeErr := results.Close()
		if execErr != nil {
			return fmt.Errorf("%s.Store.Add: execute upsert batch: %w", s.provider, execErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%s.Store.Add: close upsert batch: %w", s.provider, closeErr)
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

// Search embeds the query, runs an ANN search, and returns the matching
// documents above the configured MinScore threshold.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("%s.Store.Search: %w", s.provider, err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("%s.Store.Search: embed query: %w", s.provider, err)
	}
	queryVec := pgvec.NewVector(vectorconv.Float32(vector))

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
		return nil, fmt.Errorf("%s.Store.Search: query %s: %w", s.provider, s.fullTable, err)
	}
	defer rows.Close()

	docs = make([]vectorstore.Match, 0, req.TopK)
	for rows.Next() {
		var (
			id       string
			content  *string
			metaRaw  []byte
			distance float64
		)
		if err = rows.Scan(&id, &content, &metaRaw, &distance); err != nil {
			return nil, fmt.Errorf("%s.Store.Search: scan row: %w", s.provider, err)
		}

		score := s.distanceToScore(distance)
		if score < req.MinScore {
			continue
		}
		if id == "" {
			return nil, fmt.Errorf("%s.Store.Search: row is missing document ID", s.provider)
		}
		if content == nil || *content == "" {
			return nil, fmt.Errorf("%s.Store.Search: document %q is missing text", s.provider, id)
		}

		doc := &document.Document{ID: id, Text: *content}
		if len(metaRaw) > 0 {
			if doc.Metadata, err = unmarshalMetadata(metaRaw); err != nil {
				return nil, fmt.Errorf("%s.Store.Search: unmarshal metadata for %q: %w", s.provider, id, err)
			}
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("%s.Store.Search: read rows: %w", s.provider, err)
	}
	return docs, nil
}

// DeleteWhere removes every row whose metadata matches the predicate.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("%s.Store.DeleteWhere: %w", s.provider, err)
	}

	var (
		fragment string
		args     []any
	)
	fragment, args, err = s.buildWhereClause(expr)
	if err != nil {
		return err
	}
	if fragment == "" {
		return fmt.Errorf("%s.Store.DeleteWhere: filter produced no SQL predicate", s.provider)
	}

	sql := fmt.Sprintf(`DELETE FROM %s%s`, s.fullTable, fragment)
	if _, err = s.pool.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("%s.Store.DeleteWhere: delete from %s: %w", s.provider, s.fullTable, err)
	}
	return nil
}

// DeleteIDs removes rows by primary key — `DELETE ... WHERE id = ANY($1)`.
// pgx maps the []string to a Postgres text array. An empty slice is a
// no-op; unknown ids are silently ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	sql := fmt.Sprintf(`DELETE FROM %s WHERE id = ANY($1)`, s.fullTable)
	if _, err = s.pool.Exec(ctx, sql, ids); err != nil {
		return fmt.Errorf("%s.Store.DeleteIDs: delete from %s: %w", s.provider, s.fullTable, err)
	}
	return nil
}

// buildWhereClause converts the optional filter expression into a SQL
// fragment (prefixed with " WHERE ") and the matching argument slice.
// Returns ("", nil, nil) when filter is nil.
func (s *Store) buildWhereClause(filter filter.Predicate) (string, []any, error) {
	if filter == nil {
		return "", nil, nil
	}
	compiler := pgfilter.NewCompiler(s.metadataColumn)
	if err := compiler.Visit(filter); err != nil {
		return "", nil, fmt.Errorf("%s: compile metadata filter: %w", s.provider, err)
	}
	fragment, args := compiler.Result()
	if fragment == "" {
		return "", nil, nil
	}
	return " WHERE " + fragment, args, nil
}

func (s *Store) Close() error { return nil }

// marshalMetadata serializes the document metadata into the JSON bytes
// stored in the jsonb column. nil maps round-trip as JSON null.
func marshalMetadata(m metadata.Map) ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	return json.Marshal(m)
}

// unmarshalMetadata reverses marshalMetadata. NULL jsonb columns
// produce a nil map.
func unmarshalMetadata(b []byte) (metadata.Map, error) {
	if len(b) == 0 || string(b) == "null" {
		return nil, nil
	}
	var out metadata.Map
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
