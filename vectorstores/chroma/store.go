package chroma

import (
	"context"
	"errors"
	"fmt"

	v2 "github.com/amikos-tech/chroma-go/pkg/api/v2"
	chromaEmbed "github.com/amikos-tech/chroma-go/pkg/embeddings"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
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

// VectorStoreConfig contains configuration options for the Chroma vector store.
type VectorStoreConfig struct {
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
	DocumentBatcher document.Batcher

	// StoreDocumentContent determines whether to store and retrieve the original
	// document text in Chroma's native text field.
	// Optional: defaults to false.
	StoreDocumentContent bool
}

func (c *VectorStoreConfig) Validate() error {
	if c == nil {
		return errors.New("chroma: config is nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Client == nil {
		return errors.New("chroma: client is required")
	}
	if c.CollectionName == "" {
		return errors.New("chroma: collection name is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("chroma: embedding model is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("chroma: document batcher is required")
	}
	if c.DistanceMetric == "" {
		c.DistanceMetric = DistanceCosine
	}
	return nil
}

var _ vectorstore.VectorStore = (*VectorStore)(nil)

// VectorStore is a Chroma-backed implementation of vectorstore.VectorStore.
type VectorStore struct {
	client               v2.Client
	collection           v2.Collection
	collectionName       string
	embeddingModel       embedding.Model
	embeddingClient      *embedding.Client
	documentBatcher      document.Batcher
	distanceMetric       DistanceMetric
	storeDocumentContent bool
}

// NewVectorStore creates and initializes a Chroma VectorStore from config.
func NewVectorStore(config *VectorStoreConfig) (*VectorStore, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClientWithModel(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to create embedding client: %w", err)
	}

	store := &VectorStore{
		client:               config.Client,
		collectionName:       config.CollectionName,
		embeddingModel:       config.EmbeddingModel,
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

func (v *VectorStore) initialize(ctx context.Context, initializeSchema bool) error {
	var (
		col v2.Collection
		err error
	)

	if initializeSchema {
		col, err = v.client.GetOrCreateCollection(
			ctx,
			v.collectionName,
			v2.WithHNSWSpaceCreate(chromaEmbed.DistanceMetric(v.distanceMetric)),
		)
	} else {
		col, err = v.client.GetCollection(ctx, v.collectionName)
	}
	if err != nil {
		return fmt.Errorf("chroma: failed to get/create collection %s: %w", v.collectionName, err)
	}

	v.collection = col
	return nil
}

// distanceToScore converts a Chroma distance value into a similarity score
// in which higher values indicate greater similarity.
//
// Chroma returns distances (lower = more similar) for cosine and L2 metrics.
// For IP it returns inner-product values (higher = more similar).
func (v *VectorStore) distanceToScore(distance float64) float64 {
	switch v.distanceMetric {
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
func (v *VectorStore) buildAddOptions(docs []*document.Document, vectors [][]float64) ([]v2.CollectionAddOption, error) {
	ids := make([]v2.DocumentID, 0, len(docs))
	embs := make([]chromaEmbed.Embedding, 0, len(docs))
	metadatas := make([]v2.DocumentMetadata, 0, len(docs))
	texts := make([]string, 0, len(docs))

	for i, doc := range docs {
		ids = append(ids, v2.DocumentID(uuid.NewString()))

		f32 := math.ConvertSlice[float64, float32](vectors[i])
		embs = append(embs, chromaEmbed.NewEmbeddingFromFloat32(f32))

		meta, err := v2.NewDocumentMetadataFromMap(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("chroma: failed to convert metadata for document %d: %w", i, err)
		}
		metadatas = append(metadatas, meta)

		if v.storeDocumentContent {
			texts = append(texts, doc.Text)
		}
	}

	opts := []v2.CollectionAddOption{
		v2.WithIDs(ids...),
		v2.WithEmbeddings(embs...),
		v2.WithMetadatas(metadatas...),
	}
	if v.storeDocumentContent {
		opts = append(opts, v2.WithTexts(texts...))
	}

	return opts, nil
}

// Create embeds the documents in req and upserts them into Chroma.
func (v *VectorStore) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("chroma: invalid create request: %w", err)
	}

	batchedDocs, err := v.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("chroma: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := v.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("chroma: failed to generate embeddings: %w", err)
		}

		opts, err := v.buildAddOptions(docs, vectors)
		if err != nil {
			return err
		}

		if err = v.collection.Upsert(ctx, opts...); err != nil {
			return fmt.Errorf("chroma: failed to upsert documents into collection %s: %w",
				v.collectionName, err)
		}
	}

	return nil
}

// buildQueryOptions assembles the Query options for the given retrieval request
// and the pre-computed query embedding vector.
func (v *VectorStore) buildQueryOptions(req *vectorstore.RetrievalRequest, queryVector []float32) ([]v2.CollectionQueryOption, error) {
	includes := []v2.Include{v2.IncludeMetadatas, v2.IncludeDistances}
	if v.storeDocumentContent {
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
func (v *VectorStore) buildDocumentsFromResult(result v2.QueryResult, minScore float64) []*document.Document {
	idGroups := result.GetIDGroups()
	if len(idGroups) == 0 {
		return nil
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

	docs := make([]*document.Document, 0, len(ids))
	for i, id := range ids {
		var distance float64
		if i < len(distGroup) {
			distance = float64(distGroup[i])
		}

		score := v.distanceToScore(distance)
		if score < minScore {
			continue
		}

		doc := &document.Document{
			ID:    string(id),
			Score: score,
		}

		if v.storeDocumentContent && i < len(docGroup) && docGroup[i] != nil {
			doc.Text = docGroup[i].ContentString()
		}

		if i < len(metaGroup) && metaGroup[i] != nil {
			doc.Metadata = metadataToMap(metaGroup[i])
		}

		docs = append(docs, doc)
	}

	return docs
}

// Retrieve embeds the query in req, searches Chroma, and returns matching documents.
func (v *VectorStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("chroma: invalid retrieval request: %w", err)
	}

	vector, _, err := v.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to embed query text: %w", err)
	}

	queryVector := math.ConvertSlice[float64, float32](vector)

	opts, err := v.buildQueryOptions(req, queryVector)
	if err != nil {
		return nil, err
	}

	result, err := v.collection.Query(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("chroma: failed to query collection %s: %w", v.collectionName, err)
	}

	return v.buildDocumentsFromResult(result, req.MinScore), nil
}

// Delete removes documents from the collection that match the filter in req.
func (v *VectorStore) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("chroma: invalid delete request: %w", err)
	}

	filter, err := ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("chroma: failed to convert filter: %w", err)
	}

	var opts []v2.CollectionDeleteOption
	if filter != nil {
		opts = append(opts, v2.WithWhere(filter))
	}

	if err = v.collection.Delete(ctx, opts...); err != nil {
		return fmt.Errorf("chroma: failed to delete documents from collection %s: %w",
			v.collectionName, err)
	}

	return nil
}

// Info returns metadata about this store instance.
func (v *VectorStore) Info() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: v.client,
		Provider:     Provider,
	}
}

// Close releases resources held by the underlying Chroma collection handle.
func (v *VectorStore) Close() error {
	return v.collection.Close()
}
