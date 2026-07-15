package chroma

import (
	"context"
	"fmt"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbed "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "Chroma"

// DistanceMetric defines the distance function used by the HNSW index.
type DistanceMetric string

const (
	// DistanceCosine uses cosine distance (1 - cosine_similarity).
	// Returned distances are in [0, 2]; lower means more similar.
	DistanceCosine DistanceMetric = "cosine"
	// DistanceL2 uses squared L2 (Euclidean) distance.
	// Returned distances are in [0, ∞); lower means more similar.
	DistanceL2 DistanceMetric = "l2"
	// DistanceIP uses inner product (dot product) distance.
	// Returned values are in (-∞, ∞); higher means more similar.
	DistanceIP DistanceMetric = "ip"
)

// StoreConfig contains configuration options for the Chroma vector store.
type StoreConfig struct {
	// Context is the context for initialization operations.
	// Optional: defaults to context.Background() if nil.
	Context context.Context

	// Client is the Chroma HTTP client.
	// Required: must be provided, otherwise initialization will fail.
	Client v2.Client

	// CollectionName is the name of the Chroma collection to use.
	// Required: must be a non-empty string.
	CollectionName string

	// InitializeSchema indicates whether to automatically create the collection
	// if it does not exist. When true, GetOrCreateCollection is used; otherwise
	// the collection must already exist.
	// Optional: defaults to false.
	InitializeSchema bool

	// DistanceMetric is the HNSW distance function applied when the collection
	// is created via InitializeSchema. Has no effect on an existing collection.
	// Optional: defaults to DistanceCosine.
	DistanceMetric DistanceMetric

	// EmbeddingModel is the model used to generate vector embeddings from text.
	// Required: must be provided.
	EmbeddingModel embedding.Model

	// DocumentBatcher is responsible for batching documents before insertion.
	// Required: must be provided.
	DocumentBatcher vectorstores.Batcher

	// StoreDocumentContent determines whether to store and retrieve the original
	// document text in Chroma's native text field.
	// Optional: defaults to false.
	StoreDocumentContent bool
}

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return ErrMissingClient
	}
	if c.CollectionName == "" {
		return ErrMissingCollectionName
	}
	if c.EmbeddingModel == nil {
		return ErrMissingEmbeddingModel
	}
	if c.DocumentBatcher == nil {
		return ErrMissingDocumentBatcher
	}
	return nil
}

// ApplyDefaults fills zero fields. Context defaults to
// [context.Background]; DistanceMetric defaults to [DistanceCosine].
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.DistanceMetric == "" {
		c.DistanceMetric = DistanceCosine
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

// Store is a Chroma-backed implementation of vectorstore capability interfaces.
type Store struct {
	client               v2.Client
	collection           v2.Collection
	collectionName       string
	embeddingClient      *embeddingclient.Client
	documentBatcher      vectorstores.Batcher
	distanceMetric       DistanceMetric
	storeDocumentContent bool
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:               config.Client,
		collectionName:       config.CollectionName,
		embeddingClient:      embeddingClient,
		documentBatcher:      config.DocumentBatcher,
		distanceMetric:       config.DistanceMetric,
		storeDocumentContent: config.StoreDocumentContent,
	}

	if err = store.initialize(config.Context, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("chroma: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (s *Store) initialize(ctx context.Context, initializeSchema bool) error {
	var (
		col v2.Collection
		err error
	)

	if initializeSchema {
		col, err = s.client.GetOrCreateCollection(
			ctx,
			s.collectionName,
			v2.WithHNSWSpaceCreate(chromaEmbed.DistanceMetric(s.distanceMetric)),
		)
	} else {
		col, err = s.client.GetCollection(ctx, s.collectionName)
	}
	if err != nil {
		return fmt.Errorf("chroma: failed to get/create collection %s: %w", s.collectionName, err)
	}

	s.collection = col
	return nil
}

// distanceToScore converts a Chroma distance value into a similarity score
// in which higher values indicate greater similarity.
//
// Chroma returns distances (lower = more similar) for cosine and L2 metrics.
// For IP it returns inner-product values (higher = more similar).
func (s *Store) distanceToScore(distance float64) float64 {
	switch s.distanceMetric {
	case DistanceCosine:
		// cosine distance ≈ 1 − cosine_similarity; maps [0, 2] → [1, -1]
		return 1.0 - distance
	case DistanceL2:
		// squared L2 is unbounded; map to (0, 1] so MinScore comparisons are intuitive
		return 1.0 / (1.0 + distance)
	case DistanceIP:
		// inner product: higher already means more similar
		return distance
	default:
		return 1.0 - distance
	}
}

// metadataToMap converts a Chroma DocumentMetadata into a plain map.
// It type-asserts to the concrete *DocumentMetadataImpl to access Keys() and
// the typed getters, preserving the original value types.
func metadataToMap(meta v2.DocumentMetadata) map[string]any {
	if meta == nil {
		return nil
	}
	impl, ok := meta.(*v2.DocumentMetadataImpl)
	if !ok {
		return nil
	}
	keys := impl.Keys()
	if len(keys) == 0 {
		return nil
	}
	result := make(map[string]any, len(keys))
	for _, k := range keys {
		if s, ok := impl.GetString(k); ok {
			result[k] = s
		} else if i, ok := impl.GetInt(k); ok {
			result[k] = i
		} else if f, ok := impl.GetFloat(k); ok {
			result[k] = f
		} else if b, ok := impl.GetBool(k); ok {
			result[k] = b
		} else if sa, ok := impl.GetStringArray(k); ok {
			result[k] = sa
		} else if ia, ok := impl.GetIntArray(k); ok {
			result[k] = ia
		} else if fa, ok := impl.GetFloatArray(k); ok {
			result[k] = fa
		} else if ba, ok := impl.GetBoolArray(k); ok {
			result[k] = ba
		}
	}
	return result
}

// buildAddOptions assembles the Upsert options for a single document batch
// together with their pre-computed embedding vectors.
func (s *Store) buildAddOptions(docs []*document.Document, vectors [][]float64) ([]v2.CollectionAddOption, error) {
	ids := make([]v2.DocumentID, 0, len(docs))
	embs := make([]chromaEmbed.Embedding, 0, len(docs))
	metadatas := make([]v2.DocumentMetadata, 0, len(docs))
	texts := make([]string, 0, len(docs))

	for i, doc := range docs {
		ids = append(ids, v2.DocumentID(uuid.NewString()))

		f32 := math.ConvertSlice[float64, float32](vectors[i])
		embs = append(embs, chromaEmbed.NewEmbeddingFromFloat32(f32))

		metadataValues, err := doc.Metadata.Values()
		if err != nil {
			return nil, fmt.Errorf("chroma: decode metadata for document %d: %w", i, err)
		}
		meta, err := v2.NewDocumentMetadataFromMap(metadataValues)
		if err != nil {
			return nil, fmt.Errorf("chroma: failed to convert metadata for document %d: %w", i, err)
		}
		metadatas = append(metadatas, meta)

		if s.storeDocumentContent {
			texts = append(texts, doc.Text)
		}
	}

	opts := []v2.CollectionAddOption{
		v2.WithIDs(ids...),
		v2.WithEmbeddings(embs...),
		v2.WithMetadatas(metadatas...),
	}
	if s.storeDocumentContent {
		opts = append(opts, v2.WithTexts(texts...))
	}

	return opts, nil
}

// Add embeds the documents and upserts them into Chroma.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "chroma", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("chroma: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("chroma: failed to generate embeddings: %w", err)
		}

		opts, err := s.buildAddOptions(docs, vectors)
		if err != nil {
			return err
		}

		if err = s.collection.Upsert(ctx, opts...); err != nil {
			return fmt.Errorf("chroma: failed to upsert documents into collection %s: %w",
				s.collectionName, err)
		}
	}

	return nil
}

// buildQueryOptions assembles the Query options for the given retrieval request
// and the pre-computed query embedding vector.
func (s *Store) buildQueryOptions(req vectorstore.SearchRequest, queryVector []float32) ([]v2.CollectionQueryOption, error) {
	includes := []v2.Include{v2.IncludeMetadatas, v2.IncludeDistances}
	if s.storeDocumentContent {
		includes = append(includes, v2.IncludeDocuments)
	}

	queryEmb := chromaEmbed.NewEmbeddingFromFloat32(queryVector)

	opts := []v2.CollectionQueryOption{
		v2.WithQueryEmbeddings(queryEmb),
		v2.WithNResults(req.TopK),
		v2.WithInclude(includes...),
	}

	if req.Filter != nil {
		filter, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("chroma: failed to convert filter: %w", err)
		}
		if filter != nil {
			opts = append(opts, v2.WithWhere(filter))
		}
	}

	return opts, nil
}

// buildDocumentsFromResult assembles Lynx Documents from the parallel slices
// returned by the QueryResult interface, applying the MinScore threshold.
func (s *Store) buildDocumentsFromResult(result v2.QueryResult, minScore float64) ([]vectorstore.Match, error) {
	idGroups := result.GetIDGroups()
	if len(idGroups) == 0 {
		return nil, nil
	}

	ids := idGroups[0]

	var docGroup v2.Documents
	if dg := result.GetDocumentsGroups(); len(dg) > 0 {
		docGroup = dg[0]
	}

	var metaGroup v2.DocumentMetadatas
	if mg := result.GetMetadatasGroups(); len(mg) > 0 {
		metaGroup = mg[0]
	}

	var distGroup chromaEmbed.Distances
	if dg := result.GetDistancesGroups(); len(dg) > 0 {
		distGroup = dg[0]
	}

	docs := make([]vectorstore.Match, 0, len(ids))
	for i, id := range ids {
		var distance float64
		if i < len(distGroup) {
			distance = float64(distGroup[i])
		}

		score := s.distanceToScore(distance)
		if score < minScore {
			continue
		}

		doc := &document.Document{ID: string(id)}

		if s.storeDocumentContent && i < len(docGroup) && docGroup[i] != nil {
			doc.Text = docGroup[i].ContentString()
		}

		if i < len(metaGroup) && metaGroup[i] != nil {
			var err error
			doc.Metadata, err = metadata.FromValues(metadataToMap(metaGroup[i]))
			if err != nil {
				return nil, fmt.Errorf("chroma: encode metadata: %w", err)
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}

	return docs, nil
}

// Search embeds the query, searches Chroma, and returns matching documents.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("chroma: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "chroma", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to embed query text: %w", err)
	}

	queryVector := math.ConvertSlice[float64, float32](vector)

	var opts []v2.CollectionQueryOption
	opts, err = s.buildQueryOptions(req, queryVector)
	if err != nil {
		return nil, err
	}

	var result v2.QueryResult
	result, err = s.collection.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to query collection %s: %w", s.collectionName, err)
	}

	docs, err = s.buildDocumentsFromResult(result, req.MinScore)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// Delete removes documents from the collection that match the filter in req.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "chroma")
	defer func() { tracing.Finish(span, err) }()

	var filter v2.WhereFilter
	filter, err = ToFilter(expr)
	if err != nil {
		return fmt.Errorf("chroma: failed to convert filter: %w", err)
	}

	var opts []v2.CollectionDeleteOption
	if filter != nil {
		opts = append(opts, v2.WithWhere(filter))
	}

	if err = s.collection.Delete(ctx, opts...); err != nil {
		return fmt.Errorf("chroma: failed to delete documents from collection %s: %w",
			s.collectionName, err)
	}

	return nil
}

// DeleteIDs removes documents from the collection by their Chroma IDs.
// An empty slice is a no-op; unknown ids are silently ignored. Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "chroma")
	defer func() { tracing.Finish(span, err) }()

	docIDs := make([]v2.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = v2.DocumentID(id)
	}

	if err = s.collection.Delete(ctx, v2.WithIDs(docIDs...)); err != nil {
		return fmt.Errorf("chroma: failed to delete documents by ids from collection %s: %w",
			s.collectionName, err)
	}

	return nil
}

// Info returns metadata about this store instance.

// Close releases resources held by the underlying Chroma collection handle.
func (s *Store) Close() error {
	return s.collection.Close()
}
