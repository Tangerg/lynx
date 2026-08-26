package chroma

import (
	"context"
	"fmt"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbed "github.com/amikos-tech/chroma-go/pkg/embeddings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
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

// score converts a Chroma distance value into a similarity score in which
// higher values indicate greater similarity.
func (d DistanceMetric) score(distance float64) vectorstore.Score {
	switch d {
	case DistanceCosine:
		return vectorstore.ScoreFromCosineDistance(distance)
	case DistanceL2:
		return vectorstore.ScoreFromDistance(distance)
	case DistanceIP:
		// Chroma reports 1 - inner product as a distance.
		return vectorstore.ScoreFromOneMinusInnerProductDistance(distance)
	default:
		return vectorstore.ScoreFromValue(distance)
	}
}

// StoreConfig contains configuration options for the Chroma vector store.
type StoreConfig struct {
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
	DocumentBatcher vectorstore.Batcher
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Client == nil {
		return ErrMissingClient
	}
	if s.CollectionName == "" {
		return ErrMissingCollectionName
	}
	if s.EmbeddingModel == nil {
		return ErrMissingEmbeddingModel
	}
	if s.DocumentBatcher == nil {
		return ErrMissingDocumentBatcher
	}
	switch s.DistanceMetric {
	case DistanceCosine, DistanceL2, DistanceIP:
	default:
		return fmt.Errorf("chroma: unsupported DistanceMetric %q", s.DistanceMetric)
	}
	return nil
}

// applyDefaults fills zero fields. DistanceMetric defaults to
// [DistanceCosine].
func (s *StoreConfig) applyDefaults() {
	if s.DistanceMetric == "" {
		s.DistanceMetric = DistanceCosine
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
	client          v2.Client
	collection      v2.Collection
	collectionName  string
	embeddingClient embeddingclient.Client
	documentBatcher vectorstore.Batcher
	distanceMetric  DistanceMetric
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("chroma: create embedding client: %w", err)
	}

	store := &Store{
		client:          config.Client,
		collectionName:  config.CollectionName,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		distanceMetric:  config.DistanceMetric,
	}

	if err = store.initialize(ctx, config.InitializeSchema); err != nil {
		return nil, fmt.Errorf("chroma: initialize vector store: %w", err)
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
		return fmt.Errorf("chroma: get/create collection %s: %w", s.collectionName, err)
	}

	s.collection = col
	return nil
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
		ids = append(ids, v2.DocumentID(doc.ID))

		f32 := embedding.Float32Vector(vectors[i])
		embs = append(embs, chromaEmbed.NewEmbeddingFromFloat32(f32))

		metadataValues, err := doc.Metadata.Values()
		if err != nil {
			return nil, fmt.Errorf("chroma: decode metadata for document %d: %w", i, err)
		}
		meta, err := v2.NewDocumentMetadataFromMap(metadataValues)
		if err != nil {
			return nil, fmt.Errorf("chroma: convert metadata for document %d: %w", i, err)
		}
		metadatas = append(metadatas, meta)

		texts = append(texts, doc.Text)
	}

	opts := []v2.CollectionAddOption{
		v2.WithIDs(ids...),
		v2.WithEmbeddings(embs...),
		v2.WithMetadatas(metadatas...),
		v2.WithTexts(texts...),
	}

	return opts, nil
}

// Index embeds the documents and upserts them into Chroma.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return fmt.Errorf("chroma.Store.Index: %w", validateErr)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("chroma: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("chroma: embed documents: %w", err)
		}

		opts, err := s.buildAddOptions(docs, vectors)
		if err != nil {
			return err
		}

		if err = s.collection.Upsert(ctx, opts...); err != nil {
			return fmt.Errorf("chroma: upsert documents into collection %s: %w",
				s.collectionName, err)
		}
	}

	return nil
}

// buildQueryOptions assembles the Query options for the given retrieval request
// and the pre-computed query embedding vector.
func (s *Store) buildQueryOptions(req *vectorstore.SearchRequest, queryVector []float32) ([]v2.CollectionQueryOption, error) {
	queryEmb := chromaEmbed.NewEmbeddingFromFloat32(queryVector)

	opts := []v2.CollectionQueryOption{
		v2.WithQueryEmbeddings(queryEmb),
		v2.WithNResults(req.Options.TopK),
		v2.WithInclude(v2.IncludeDocuments, v2.IncludeMetadatas, v2.IncludeDistances),
	}

	if req.Options.Filter != nil {
		visitor := NewVisitor()
		if err := req.Options.Filter.Accept(visitor); err != nil {
			return nil, fmt.Errorf("chroma: convert filter: %w", err)
		}
		if result := visitor.Result(); result != nil {
			opts = append(opts, v2.WithWhere(result))
		}
	}

	return opts, nil
}

// buildDocumentsFromResult assembles Lynx Documents from the parallel slices
// returned by the QueryResult interface, applying the MinScore threshold.
func (s *Store) buildDocumentsFromResult(result v2.QueryResult, minScore vectorstore.Score) ([]*vectorstore.SearchResult, error) {
	idGroups := result.GetIDGroups()
	if len(idGroups) == 0 {
		return nil, nil
	}
	if len(idGroups) != 1 {
		return nil, fmt.Errorf("chroma: query returned %d ID groups for one query vector", len(idGroups))
	}

	ids := idGroups[0]

	var docGroup v2.Documents
	if dg := result.GetDocumentsGroups(); len(dg) > 0 {
		if len(dg) != 1 {
			return nil, fmt.Errorf("chroma: query returned %d document groups for one query vector", len(dg))
		}
		docGroup = dg[0]
	}
	if len(docGroup) != len(ids) {
		return nil, fmt.Errorf("chroma: query returned %d documents for %d IDs", len(docGroup), len(ids))
	}

	var metaGroup v2.DocumentMetadatas
	if mg := result.GetMetadatasGroups(); len(mg) > 0 {
		if len(mg) != 1 {
			return nil, fmt.Errorf("chroma: query returned %d metadata groups for one query vector", len(mg))
		}
		metaGroup = mg[0]
	}
	if len(metaGroup) != len(ids) {
		return nil, fmt.Errorf("chroma: query returned %d metadata values for %d IDs", len(metaGroup), len(ids))
	}

	var distGroup chromaEmbed.Distances
	if dg := result.GetDistancesGroups(); len(dg) > 0 {
		if len(dg) != 1 {
			return nil, fmt.Errorf("chroma: query returned %d distance groups for one query vector", len(dg))
		}
		distGroup = dg[0]
	}
	if len(distGroup) != len(ids) {
		return nil, fmt.Errorf("chroma: query returned %d distances for %d IDs", len(distGroup), len(ids))
	}

	docs := make([]*vectorstore.SearchResult, 0, len(ids))
	for i, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("chroma: query result %d is missing ID", i)
		}
		distance := float64(distGroup[i])
		score := s.distanceMetric.score(distance)
		if score < minScore {
			continue
		}

		if docGroup[i] == nil || docGroup[i].ContentString() == "" {
			return nil, fmt.Errorf("chroma: query result %d is missing document text", i)
		}
		doc := &document.Document{ID: string(id), Text: docGroup[i].ContentString()}

		if i < len(metaGroup) && metaGroup[i] != nil {
			var err error
			doc.Metadata, err = metadata.FromValues(metadataToMap(metaGroup[i]))
			if err != nil {
				return nil, fmt.Errorf("chroma: convert metadata: %w", err)
			}
		}

		docs = append(docs, &vectorstore.SearchResult{Document: doc, Score: score})
	}

	return docs, nil
}

// Search embeds the query, searches Chroma, and returns matching documents.
func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("chroma.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("chroma: embed query: %w", err)
	}

	queryVector := embedding.Float32Vector(vector)

	var opts []v2.CollectionQueryOption
	opts, err = s.buildQueryOptions(req, queryVector)
	if err != nil {
		return nil, err
	}

	var result v2.QueryResult
	result, err = s.collection.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("chroma: query collection %s: %w", s.collectionName, err)
	}

	docs, err = s.buildDocumentsFromResult(result, req.Options.MinScore)
	if err != nil {
		return nil, err
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

// Delete removes documents from the collection that match the filter in req.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("chroma.Store.DeleteWhere: %w", err)
	}

	visitor := NewVisitor()
	if err = expr.Accept(visitor); err != nil {
		return fmt.Errorf("chroma: convert filter: %w", err)
	}

	var opts []v2.CollectionDeleteOption
	if result := visitor.Result(); result != nil {
		opts = append(opts, v2.WithWhere(result))
	}

	if err = s.collection.Delete(ctx, opts...); err != nil {
		return fmt.Errorf("chroma: delete documents from collection %s: %w",
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

	docIDs := make([]v2.DocumentID, len(ids))
	for i, id := range ids {
		docIDs[i] = v2.DocumentID(id)
	}

	if err = s.collection.Delete(ctx, v2.WithIDs(docIDs...)); err != nil {
		return fmt.Errorf("chroma: delete documents by ids from collection %s: %w",
			s.collectionName, err)
	}

	return nil
}

// Info returns metadata about this store instance.

// Close releases resources held by the underlying Chroma collection handle.
func (s *Store) Close() error {
	return s.collection.Close()
}
