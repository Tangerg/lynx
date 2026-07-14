package typesense

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/typesense/typesense-go/v3/typesense"
	"github.com/typesense/typesense-go/v3/typesense/api"
	"github.com/typesense/typesense-go/v3/typesense/api/pointer"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "Typesense"

const (
	DefaultCollectionName = "lynx_vector_store"
	DefaultDimensions     = 1536
	idField               = "doc_id"
	contentField          = "content"
	metadataField         = "metadata"
	embeddingField        = "embedding"
)

// safeIdentifier matches the SQL identifier shape.

// StoreConfig contains configuration options for the Typesense vector
// store.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap. Optional;
	// defaults to context.Background().
	Context context.Context

	// Client is the typesense-go client. Required.
	Client *typesense.Client

	// CollectionName names the Typesense collection. Optional:
	// defaults to [DefaultCollectionName].
	CollectionName string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher vectorstores.Batcher

	// Dimensions sets the vector width for new collections. When
	// zero, the store asks the embedding model for its native
	// dimensionality and falls back to [DefaultDimensions].
	Dimensions int

	// InitializeSchema, when true, creates the collection with the
	// right schema if it doesn't already exist.
	InitializeSchema bool
}

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("typesense: Client is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("typesense: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("typesense: DocumentBatcher is required")
	}
	if !ident.Pattern.MatchString(c.CollectionName) {
		return fmt.Errorf("typesense: CollectionName=%q must be a safe identifier", c.CollectionName)
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.CollectionName = cmp.Or(c.CollectionName, DefaultCollectionName)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
)

// Store is a Typesense backed the vectorstore capability interfaces implementation.
type Store struct {
	client          *typesense.Client
	collectionName  string
	embeddingModel  embedding.Model
	embeddingClient *embedding.Client
	documentBatcher vectorstores.Batcher
	dimensions      int
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("typesense: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:          config.Client,
		collectionName:  config.CollectionName,
		embeddingModel:  config.EmbeddingModel,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		dimensions:      config.Dimensions,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("typesense: failed to initialize store: %w", err)
	}
	return store, nil
}

// initialize resolves dimensionality and creates the collection when
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
		return errors.New("typesense: Dimensions must be > 0")
	}

	if !initSchema {
		return nil
	}

	// Probe for an existing collection; if Retrieve succeeds we
	// assume the schema matches.
	if _, err := s.client.Collection(s.collectionName).Retrieve(ctx); err == nil {
		return nil
	}

	schema := &api.CollectionSchema{
		Name: s.collectionName,
		Fields: []api.Field{
			{Name: idField, Type: "string", Optional: pointer.False()},
			{Name: contentField, Type: "string", Optional: pointer.False()},
			{Name: metadataField, Type: "object", Optional: pointer.True()},
			{
				Name:     embeddingField,
				Type:     "float[]",
				NumDim:   pointer.Int(s.dimensions),
				Optional: pointer.False(),
			},
		},
		EnableNestedFields: pointer.True(),
	}
	if _, err := s.client.Collections().Create(ctx, schema); err != nil {
		return fmt.Errorf("typesense: create collection %s: %w", s.collectionName, err)
	}
	return nil
}

// Add embeds documents and imports them via the upsert action.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "typesense", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("typesense: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("typesense: failed to generate embeddings: %w", err)
		}

		payload := make([]any, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("typesense: decode metadata for %s: %w", id, err)
			}
			payload = append(payload, map[string]any{
				idField:        id,
				contentField:   doc.Text,
				metadataField:  metaOrEmpty(metadataValues),
				embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			})
		}

		params := &api.ImportDocumentsParams{
			Action: pointer.Any(api.Upsert),
		}
		if _, err := s.client.Collection(s.collectionName).Documents().Import(ctx, payload, params); err != nil {
			return fmt.Errorf("typesense: import documents: %w", err)
		}
	}
	return nil
}

// Search runs a vector search via the documents.Search API.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("typesense: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "typesense", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("typesense: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)
	vectorQuery := formatVectorQuery(queryVec, req.TopK)

	filterBy, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	params := &api.SearchCollectionParams{
		Q:           pointer.String("*"),
		VectorQuery: pointer.String(vectorQuery),
		PerPage:     pointer.Int(req.TopK),
	}
	if filterBy != "" {
		params.FilterBy = pointer.String(filterBy)
	}

	result, err := s.client.Collection(s.collectionName).Documents().Search(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("typesense: search %s: %w", s.collectionName, err)
	}
	if result == nil || result.Hits == nil {
		return nil, nil
	}

	docs = make([]vectorstore.Match, 0, len(*result.Hits))
	for _, hit := range *result.Hits {
		match, err := toMatch(hit)
		if err != nil {
			return nil, err
		}
		if match.Score < req.MinScore {
			continue
		}
		docs = append(docs, match)
	}
	return docs, nil
}

// Delete removes documents matching the filter expression.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "typesense")
	defer func() { tracing.Finish(span, err) }()

	filterBy, err := s.buildFilter(expr)
	if err != nil {
		return err
	}
	if filterBy == "" {
		return errors.New("typesense: refusing to delete on empty filter")
	}

	params := &api.DeleteDocumentsParams{FilterBy: pointer.String(filterBy)}
	if _, err := s.client.Collection(s.collectionName).Documents().Delete(ctx, params); err != nil {
		return fmt.Errorf("typesense: delete: %w", err)
	}
	return nil
}

func (s *Store) buildFilter(filter filter.Expr) (string, error) {
	if filter == nil {
		return "", nil
	}
	v := NewVisitor(metadataField)
	if err := v.Visit(filter); err != nil {
		return "", fmt.Errorf("typesense: convert filter: %w", err)
	}
	return v.Result(), nil
}

func toMatch(hit api.SearchResultHit) (vectorstore.Match, error) {
	doc := &document.Document{}
	if hit.Document == nil {
		return vectorstore.Match{Document: doc}, nil
	}
	raw := *hit.Document
	if id, ok := raw[idField].(string); ok {
		doc.ID = id
	}
	if content, ok := raw[contentField].(string); ok {
		doc.Text = content
	}
	if meta, ok := raw[metadataField].(map[string]any); ok && len(meta) > 0 {
		var err error
		doc.Metadata, err = metadata.FromValues(meta)
		if err != nil {
			return vectorstore.Match{}, fmt.Errorf("typesense: encode metadata: %w", err)
		}
	}
	// Typesense returns distance in the cosine [0, 2] range; map
	// onto a "higher = more similar" score in [0, 1].
	var matchScore float64
	if hit.VectorDistance != nil {
		distance := float64(*hit.VectorDistance)
		score := 1.0 - distance/2.0
		switch {
		case score < 0:
			matchScore = 0
		case score > 1:
			matchScore = 1
		default:
			matchScore = score
		}
	}
	return vectorstore.Match{Document: doc, Score: matchScore}, nil
}

// formatVectorQuery builds the Typesense `vector_query` string —
// "embedding:([f1,f2,...], k: N)".
func formatVectorQuery(vec []float32, topK int) string {
	var b strings.Builder
	b.Grow(len(vec) * 6)
	b.WriteString(embeddingField)
	b.WriteString(":([")
	for i, f := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteString("], k: ")
	b.WriteString(strconv.Itoa(topK))
	b.WriteByte(')')
	return b.String()
}

func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (s *Store) Close() error { return nil }
