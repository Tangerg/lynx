package redis

import (
	"cmp"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	stdmath "math"
	"strconv"
	"strings"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "Redis"

const (
	DefaultIndexName       = "lynx-vector-index"
	DefaultKeyPrefix       = "embedding:"
	DefaultContentField    = "content"
	DefaultEmbeddingField  = "embedding"
	DefaultMetadataPrefix  = "" // empty: metadata keys land at top level of the HASH
	DefaultDimensions      = 1536
	DefaultDistanceMetric  = DistanceCosine
	DefaultIndexAlgorithm  = AlgorithmHNSW
	DefaultHNSWM           = 16
	DefaultHNSWEFConstruct = 200
	DefaultHNSWEFRuntime   = 10
	distanceFieldName      = "__vector_distance"
	vectorParamName        = "lynx_query_vec"
)

// DistanceMetric selects the similarity function used by the
// RediSearch vector index.
type DistanceMetric string

const (
	// DistanceCosine — cosine distance, range [0, 2]. The store
	// transforms it into a [0, 1] similarity score where higher is
	// more similar.
	DistanceCosine DistanceMetric = "COSINE"

	// DistanceL2 — Euclidean distance, range [0, ∞).
	DistanceL2 DistanceMetric = "L2"

	// DistanceIP — inner product. RediSearch returns the inner
	// product itself; the store maps it onto [0, 1] for unit-norm
	// vectors via (ip+1)/2.
	DistanceIP DistanceMetric = "IP"
)

// IndexAlgorithm selects the RediSearch vector indexing algorithm.
type IndexAlgorithm string

const (
	// AlgorithmHNSW — hierarchical navigable small-world graph.
	// Default; best query performance.
	AlgorithmHNSW IndexAlgorithm = "HNSW"

	// AlgorithmFlat — exhaustive (brute-force) search. Useful for
	// small collections where build / memory cost matters more than
	// query latency.
	AlgorithmFlat IndexAlgorithm = "FLAT"
)

// MetadataFieldType names the RediSearch schema field types the store
// understands. Callers declare these up-front so the filter visitor
// can validate field names and pick the right query syntax.
type MetadataFieldType int

const (
	// FieldTag — RediSearch TAG field. Exact-match on categorical
	// data; supports IN / != via "|" join and "-" prefix.
	FieldTag MetadataFieldType = iota + 1

	// FieldText — full-text indexed field.
	FieldText

	// FieldNumeric — numeric range field.
	FieldNumeric
)

// MetadataField declares one filterable metadata key. the framework's
// builder calls this a "MetadataField".
type MetadataField struct {
	// Name is the HASH field / JSON key that holds the value.
	Name string

	// Type controls the RediSearch index field type. See
	// [FieldTag] / [FieldText] / [FieldNumeric].
	Type MetadataFieldType

	// Sortable, when true, marks the field SORTABLE in the schema.
	Sortable bool
}

// StoreConfig contains configuration options for the Redis vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to [context.Background].
	Context context.Context

	// Client is the go-redis client (single, cluster, or sentinel).
	// Required.
	Client goredis.UniversalClient

	// IndexName names the RediSearch index. Optional: defaults to
	// [DefaultIndexName].
	IndexName string

	// KeyPrefix is the Redis-key prefix the index attaches to —
	// every stored HASH lives at `<KeyPrefix><id>`. Optional:
	// defaults to [DefaultKeyPrefix].
	KeyPrefix string

	// ContentField is the HASH field that holds the original
	// document text. Optional: defaults to [DefaultContentField].
	ContentField string

	// EmbeddingField is the HASH field that holds the binary
	// FLOAT32 vector. Optional: defaults to [DefaultEmbeddingField].
	EmbeddingField string

	// MetadataFields enumerates every metadata key the index should
	// understand. Only declared fields can appear in a filter
	// expression — the store rejects unknown identifiers up-front to
	// preclude query injection.
	MetadataFields []MetadataField

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher vectorstores.Batcher

	// Dimensions sets the vector width registered with the index.
	// When zero the store asks the embedding model for its native
	// dimensionality and falls back to [DefaultDimensions].
	Dimensions int

	// DistanceMetric selects the vector similarity function.
	// Optional: defaults to [DistanceCosine].
	DistanceMetric DistanceMetric

	// IndexAlgorithm selects HNSW vs FLAT. Optional: defaults to
	// [AlgorithmHNSW].
	IndexAlgorithm IndexAlgorithm

	// HNSWM / HNSWEFConstruct / HNSWEFRuntime tune the HNSW index.
	// Each defaults via [DefaultHNSW*] when zero. Ignored when
	// IndexAlgorithm is FLAT.
	HNSWM           int
	HNSWEFConstruct int
	HNSWEFRuntime   int

	// InitializeSchema, when true, runs FT.CREATE on construction if
	// the index doesn't already exist. When false, the store assumes
	// the index is pre-provisioned.
	InitializeSchema bool
}

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("redis: Client is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("redis: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("redis: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.IndexName = cmp.Or(c.IndexName, DefaultIndexName)
	c.KeyPrefix = cmp.Or(c.KeyPrefix, DefaultKeyPrefix)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DefaultDistanceMetric)
	c.IndexAlgorithm = cmp.Or(c.IndexAlgorithm, DefaultIndexAlgorithm)
	if c.IndexAlgorithm == AlgorithmHNSW {
		if c.HNSWM == 0 {
			c.HNSWM = DefaultHNSWM
		}
		if c.HNSWEFConstruct == 0 {
			c.HNSWEFConstruct = DefaultHNSWEFConstruct
		}
		if c.HNSWEFRuntime == 0 {
			c.HNSWEFRuntime = DefaultHNSWEFRuntime
		}
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store is a Redis-backed implementation of the vectorstore capability interfaces. It
// stores documents as Redis HASHes and queries them through RediSearch
// vector + metadata indexes.
type Store struct {
	client          goredis.UniversalClient
	indexName       string
	keyPrefix       string
	contentField    string
	embeddingField  string
	metadataFields  []MetadataField
	fieldTypes      map[string]MetadataFieldType
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher vectorstores.Batcher
	dimensions      int
	distanceMetric  DistanceMetric
	indexAlgorithm  IndexAlgorithm
	hnswM           int
	hnswEFConstruct int
	hnswEFRuntime   int
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("redis: failed to create embedding client: %w", err)
	}

	fieldTypes := make(map[string]MetadataFieldType, len(config.MetadataFields))
	for _, f := range config.MetadataFields {
		if f.Name == "" {
			return nil, errors.New("redis: MetadataField.Name must not be empty")
		}
		fieldTypes[f.Name] = f.Type
	}

	store := &Store{
		client:          config.Client,
		indexName:       config.IndexName,
		keyPrefix:       config.KeyPrefix,
		contentField:    config.ContentField,
		embeddingField:  config.EmbeddingField,
		metadataFields:  config.MetadataFields,
		fieldTypes:      fieldTypes,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
		distanceMetric:  config.DistanceMetric,
		indexAlgorithm:  config.IndexAlgorithm,
		hnswM:           config.HNSWM,
		hnswEFConstruct: config.HNSWEFConstruct,
		hnswEFRuntime:   config.HNSWEFRuntime,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("redis: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves the vector dimensionality and creates the
// RediSearch index when requested.
func (s *Store) initialize(ctx context.Context, initSchema bool) error {
	if s.dimensions <= 0 {
		if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
			s.dimensions = int(dim)
		} else {
			s.dimensions = DefaultDimensions
		}
	}
	if s.dimensions <= 0 {
		return errors.New("redis: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}

	// FT._LIST returns existing index names — skip creation when ours
	// is already there.
	existing, err := s.client.FT_List(ctx).Result()
	if err != nil {
		return fmt.Errorf("FT._LIST: %w", err)
	}
	for _, name := range existing {
		if name == s.indexName {
			return nil
		}
	}

	schema := s.buildSchema()
	opts := &goredis.FTCreateOptions{
		OnHash: true,
		Prefix: []any{s.keyPrefix},
	}
	if _, err = s.client.FTCreate(ctx, s.indexName, opts, schema...).Result(); err != nil {
		return fmt.Errorf("FT.CREATE %s: %w", s.indexName, err)
	}
	return nil
}

func (s *Store) buildSchema() []*goredis.FieldSchema {
	schema := []*goredis.FieldSchema{
		{
			FieldName: s.contentField,
			FieldType: goredis.SearchFieldTypeText,
			Weight:    1.0,
		},
		{
			FieldName:  s.embeddingField,
			FieldType:  goredis.SearchFieldTypeVector,
			VectorArgs: s.vectorArgs(),
		},
	}

	for _, f := range s.metadataFields {
		fs := &goredis.FieldSchema{
			FieldName: f.Name,
			FieldType: searchFieldType(f.Type),
			Sortable:  f.Sortable,
		}
		schema = append(schema, fs)
	}
	return schema
}

func (s *Store) vectorArgs() *goredis.FTVectorArgs {
	args := &goredis.FTVectorArgs{}
	switch s.indexAlgorithm {
	case AlgorithmFlat:
		args.FlatOptions = &goredis.FTFlatOptions{
			Type:           "FLOAT32",
			Dim:            s.dimensions,
			DistanceMetric: string(s.distanceMetric),
		}
	case AlgorithmHNSW:
		fallthrough
	default:
		args.HNSWOptions = &goredis.FTHNSWOptions{
			Type:            "FLOAT32",
			Dim:             s.dimensions,
			DistanceMetric:  string(s.distanceMetric),
			MaxEdgesPerNode: s.hnswM,
			EFRunTime:       s.hnswEFRuntime,
		}
	}
	return args
}

func searchFieldType(t MetadataFieldType) goredis.SearchFieldType {
	switch t {
	case FieldNumeric:
		return goredis.SearchFieldTypeNumeric
	case FieldText:
		return goredis.SearchFieldTypeText
	case FieldTag:
		fallthrough
	default:
		return goredis.SearchFieldTypeTag
	}
}

// distanceToScore maps the raw distance returned by RediSearch to a
// "higher = more similar" score in [0, 1]. Mirrors RedisVL's
// implementation referenced by the framework.
func (s *Store) distanceToScore(distance float64) float64 {
	switch s.distanceMetric {
	case DistanceL2:
		return 1.0 / (1.0 + distance)
	case DistanceIP:
		// pgvector-style IP returns the inner product; clamp via
		// (ip+1)/2 for unit vectors, then squeeze through stdmath
		// to keep the range stable for non-normalized inputs.
		score := (distance + 1.0) / 2.0
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
		score := (2.0 - distance) / 2.0
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

// Create embeds documents and writes them as Redis HASHes keyed by
// `<KeyPrefix><id>`.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "redis", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("redis: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("redis: failed to generate embeddings: %w", err)
		}

		pipe := s.client.Pipeline()
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("redis: decode metadata for %s: %w", id, err)
			}
			fields := map[string]any{
				s.contentField:   doc.Text,
				s.embeddingField: float32sToBytes(math.ConvertSlice[float64, float32](vectors[i])),
			}
			for k, v := range metadataValues {
				fields[k] = formatMetadataValue(v)
			}
			pipe.HSet(ctx, s.keyPrefix+id, fields)
		}

		if _, err = pipe.Exec(ctx); err != nil {
			return fmt.Errorf("redis: pipeline HSET: %w", err)
		}
	}
	return nil
}

// Retrieve embeds the query, runs a KNN search through RediSearch,
// and returns the matching documents above MinScore.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("redis: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "redis", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("redis: failed to embed query: %w", err)
	}
	queryVec := float32sToBytes(math.ConvertSlice[float64, float32](vector))

	filterQuery, err := s.buildFilterQuery(req.Filter)
	if err != nil {
		return nil, err
	}

	// RediSearch hybrid syntax: <filter>=>[KNN <k> @embedding $vec AS distance]
	queryStr := fmt.Sprintf(
		"%s=>[KNN %d @%s $%s AS %s]",
		filterQuery, req.TopK, s.embeddingField, vectorParamName, distanceFieldName,
	)

	returnFields := make([]goredis.FTSearchReturn, 0, 3+len(s.metadataFields))
	returnFields = append(returnFields, goredis.FTSearchReturn{FieldName: s.contentField})
	returnFields = append(returnFields, goredis.FTSearchReturn{FieldName: distanceFieldName})
	for _, f := range s.metadataFields {
		returnFields = append(returnFields, goredis.FTSearchReturn{FieldName: f.Name})
	}

	opts := &goredis.FTSearchOptions{
		Params: map[string]any{
			vectorParamName: queryVec,
		},
		Return:         returnFields,
		LimitOffset:    0,
		Limit:          req.TopK,
		DialectVersion: 2,
		SortBy: []goredis.FTSearchSortBy{
			{FieldName: distanceFieldName, Asc: true},
		},
	}

	result, err := s.client.FTSearchWithArgs(ctx, s.indexName, queryStr, opts).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: FT.SEARCH %s: %w", s.indexName, err)
	}

	docs = make([]vectorstore.Match, 0, len(result.Docs))
	for _, hit := range result.Docs {
		score, err := s.scoreFromFields(hit.Fields)
		if err != nil {
			return nil, err
		}
		if score < req.MinScore {
			continue
		}
		doc, err := s.toDocument(hit)
		if err != nil {
			return nil, err
		}
		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}
	return docs, nil
}

// Delete looks up documents matching the filter via FT.SEARCH, then
// removes the underlying keys with DEL.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "redis")
	defer func() { tracing.Finish(span, err) }()

	var query string
	query, err = s.buildFilterQuery(expr)
	if err != nil {
		return err
	}
	if query == "*" {
		return errors.New("redis: refusing to DELETE on empty filter — pass a non-trivial expression")
	}

	const pageSize = 500
	opts := &goredis.FTSearchOptions{
		NoContent:      true,
		LimitOffset:    0,
		Limit:          pageSize,
		DialectVersion: 2,
	}
	for {
		result, err := s.client.FTSearchWithArgs(ctx, s.indexName, query, opts).Result()
		if err != nil {
			return fmt.Errorf("redis: FT.SEARCH %s: %w", s.indexName, err)
		}
		if len(result.Docs) == 0 {
			return nil
		}
		keys := make([]string, 0, len(result.Docs))
		for _, hit := range result.Docs {
			keys = append(keys, hit.ID)
		}
		if _, err = s.client.Del(ctx, keys...).Result(); err != nil {
			return fmt.Errorf("redis: DEL: %w", err)
		}
		if len(result.Docs) < pageSize {
			return nil
		}
	}
}

// DeleteIDs removes documents by id, resolving each to its HASH key
// `<KeyPrefix><id>` and issuing a single DEL. An empty slice is a
// no-op; unknown ids are silently ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "redis")
	defer func() { tracing.Finish(span, err) }()

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = s.keyPrefix + id
	}
	if _, err = s.client.Del(ctx, keys...).Result(); err != nil {
		return fmt.Errorf("redis: DEL: %w", err)
	}
	return nil
}

// buildFilterQuery turns the optional filter.Expr filter into a
// RediSearch query string. Returns "*" (match-all) when filter is nil,
// matching the syntax FT.SEARCH expects in front of the KNN tail.
func (s *Store) buildFilterQuery(filter filter.Expr) (string, error) {
	if filter == nil {
		return "*", nil
	}
	v := NewVisitor(s.fieldTypes)
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("redis: convert filter: %w", err)
	}
	fragment := v.Result()
	if fragment == "" {
		return "*", nil
	}
	return "(" + fragment + ")", nil
}

func (s *Store) scoreFromFields(fields map[string]string) (float64, error) {
	raw, ok := fields[distanceFieldName]
	if !ok {
		return 0, fmt.Errorf("redis: missing distance field %q in result", distanceFieldName)
	}
	dist, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("redis: parse distance %q: %w", raw, err)
	}
	return s.distanceToScore(dist), nil
}

func (s *Store) toDocument(hit goredis.Document) (*document.Document, error) {
	id := strings.TrimPrefix(hit.ID, s.keyPrefix)
	doc := &document.Document{
		ID:   id,
		Text: hit.Fields[s.contentField],
	}

	if len(s.metadataFields) > 0 {
		meta := make(map[string]any, len(s.metadataFields))
		for _, f := range s.metadataFields {
			if v, ok := hit.Fields[f.Name]; ok {
				meta[f.Name] = parseMetadataValue(v, f.Type)
			}
		}
		if len(meta) > 0 {
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("redis: encode metadata: %w", err)
			}
		}
	}
	return doc, nil
}

// float32sToBytes serializes a vector into the little-endian FLOAT32
// blob RediSearch expects.
func float32sToBytes(values []float32) []byte {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], stdmath.Float32bits(v))
	}
	return buf
}

// formatMetadataValue coerces a Go value into the HASH string form
// RediSearch can index. Slices and maps are JSON-encoded — they only
// matter when the caller stored them as TEXT fields.
func formatMetadataValue(v any) any {
	switch val := v.(type) {
	case nil:
		return ""
	case string, int, int64, float32, float64, bool:
		return val
	case []byte:
		return val
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprint(val)
		}
		return string(b)
	}
}

// parseMetadataValue reverses formatMetadataValue based on the schema
// type — numeric fields come back as float64, everything else stays
// a string.
func parseMetadataValue(raw string, t MetadataFieldType) any {
	if t == FieldNumeric {
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			return n
		}
	}
	return raw
}

func (s *Store) Close() error { return nil }
