package qdrant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/embedding"
	"github.com/Tangerg/lynx/ai/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/pkg/ptr"
)

const (
	Provider = "Qdrant"
)

const (
	// payloadDocumentContentKey is the payload key for saving document content
	payloadDocumentContentKey = "__payload_document_content__"
)

// VectorStoreConfig contains configuration options for Qdrant vector store.
type VectorStoreConfig struct {
	// Context is the context for all operations.
	// Optional: defaults to context.Background() if nil.
	Context context.Context

	// Client is the Qdrant client instance for communicating with Qdrant server.
	// Required: must be provided, otherwise initialization will fail.
	Client *qdrant.Client

	// CollectionName is the name of the collection to use for storing vectors.
	// Required: must be a non-empty string.
	CollectionName string

	// InitializeSchema indicates whether to automatically create the collection
	// if it does not exist. When set to true, the collection will be created
	// with vector configuration based on EmbeddingModel dimensions.
	// Optional: defaults to false.
	InitializeSchema bool

	// EmbeddingModel is the model used to generate vector embeddings from text.
	// It is also used to determine the vector dimension when creating collections.
	// Required: must be provided for both embedding generation and schema initialization.
	EmbeddingModel embedding.Model

	// DocumentBatcher is responsible for batching documents before insertion.
	// This helps optimize bulk operations and embedding generation.
	// Required: must be provided to handle document batching logic.
	DocumentBatcher document.Batcher

	// StoreDocumentContent determines whether to store the original document
	// content in the payload. When true, the full text will be saved with a
	// special key, allowing retrieval of original content without external storage.
	// Optional: defaults to false to save storage space.
	StoreDocumentContent bool
}

func (c *VectorStoreConfig) Validate() error {
	if c == nil {
		return errors.New("qdrant: config is nil")
	}

	if c.Context == nil {
		c.Context = context.Background()
	}

	if c.Client == nil {
		return errors.New("qdrant: client is required")
	}

	if c.CollectionName == "" {
		return errors.New("qdrant: collection name is required")
	}

	if c.EmbeddingModel == nil {
		return errors.New("qdrant: embedding model is required")
	}

	if c.DocumentBatcher == nil {
		return errors.New("qdrant: document batcher is required")
	}

	return nil
}

var _ vectorstore.VectorStore = (*VectorStore)(nil)

type VectorStore struct {
	config               *VectorStoreConfig
	client               *qdrant.Client
	embeddingModel       embedding.Model
	embeddingClient      *embedding.Client
	documentBatcher      document.Batcher
	collectionName       string
	initializeSchema     bool
	storeDocumentContent bool
}

func NewVectorStore(config *VectorStoreConfig) (*VectorStore, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClientWithModel(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to create embedding client: %w", err)
	}

	store := &VectorStore{
		config:               config,
		client:               config.Client,
		embeddingModel:       config.EmbeddingModel,
		embeddingClient:      embeddingClient,
		documentBatcher:      config.DocumentBatcher,
		collectionName:       config.CollectionName,
		initializeSchema:     config.InitializeSchema,
		storeDocumentContent: config.StoreDocumentContent,
	}

	if err := store.initialize(config.Context); err != nil {
		return nil, fmt.Errorf("qdrant: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (v *VectorStore) initialize(ctx context.Context) error {
	if !v.initializeSchema {
		return nil
	}

	exists, err := v.client.CollectionExists(ctx, v.collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if exists {
		return nil
	}

	err = v.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: v.collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(v.embeddingModel.Dimensions()),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to create collection %s: %w", v.collectionName, err)
	}

	return nil
}

func (v *VectorStore) buildUpsertPoints(ctx context.Context, req *vectorstore.CreateRequest) (*qdrant.UpsertPoints, error) {
	upsertPoints := &qdrant.UpsertPoints{
		CollectionName: v.collectionName,
		Wait:           ptr.Pointer(true),
	}

	batchedDocs, err := v.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return nil, fmt.Errorf("failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := v.
			embeddingClient.
			EmbedDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate vectors: %w", err)
		}

		for i, doc := range docs {
			point, err := v.buildPointStruct(doc, vectors[i])
			if err != nil {
				return nil, fmt.Errorf("failed to build point struct for document %s: %w", doc.ID, err)
			}

			upsertPoints.Points = append(upsertPoints.Points, point)
		}
	}

	return upsertPoints, nil
}

func (v *VectorStore) buildPointStruct(doc *document.Document, vector []float64) (*qdrant.PointStruct, error) {
	docID := doc.ID
	if docID == "" {
		docID = uuid.NewString()
	}

	point := &qdrant.PointStruct{
		Id: qdrant.NewID(docID),
	}

	vectorData := math.ConvertSlice[float64, float32](vector)
	point.Vectors = qdrant.NewVectors(vectorData...)

	payload, err := qdrant.TryValueMap(doc.Metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to convert metadata to payload: %w", err)
	}
	point.Payload = payload

	if v.storeDocumentContent {
		contentValue, err := qdrant.NewValue(doc.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to create content value: %w", err)
		}
		point.Payload[payloadDocumentContentKey] = contentValue
	}

	return point, nil
}

func (v *VectorStore) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("qdrant: invalid create request: %w", err)
	}

	upsertPoints, err := v.buildUpsertPoints(ctx, req)
	if err != nil {
		return err
	}

	_, err = v.client.Upsert(ctx, upsertPoints)
	if err != nil {
		return fmt.Errorf("qdrant: failed to upsert %d points to collection %s: %w",
			len(upsertPoints.Points), v.collectionName, err)
	}

	return nil
}

func (v *VectorStore) buildQueryPoints(ctx context.Context, req *vectorstore.RetrievalRequest) (*qdrant.QueryPoints, error) {
	queryPoints := &qdrant.QueryPoints{
		CollectionName: v.collectionName,
		ScoreThreshold: ptr.Pointer(float32(req.MinScore)),
		Limit:          ptr.Pointer(uint64(req.TopK)),
		WithPayload:    qdrant.NewWithPayload(true),
	}

	if req.Filter != nil {
		filter, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("failed to convert filter: %w", err)
		}
		queryPoints.Filter = filter
	}

	vector, _, err := v.embeddingClient.
		EmbedText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query text: %w", err)
	}

	queryVector := math.ConvertSlice[float64, float32](vector)
	queryPoints.Query = qdrant.NewQuery(queryVector...)

	return queryPoints, nil
}

func (v *VectorStore) convertQdrantValue(value *qdrant.Value) any {
	if value == nil {
		return nil
	}

	switch kind := value.Kind.(type) {
	case *qdrant.Value_DoubleValue:
		return kind.DoubleValue

	case *qdrant.Value_IntegerValue:
		return kind.IntegerValue

	case *qdrant.Value_StringValue:
		return kind.StringValue

	case *qdrant.Value_BoolValue:
		return kind.BoolValue

	case *qdrant.Value_NullValue:
		return nil

	case *qdrant.Value_StructValue:
		return v.convertQdrantStruct(kind.StructValue)

	case *qdrant.Value_ListValue:
		return v.convertQdrantList(kind.ListValue)

	default:
		return nil
	}
}

func (v *VectorStore) convertQdrantStruct(s *qdrant.Struct) map[string]any {
	if s == nil || s.Fields == nil {
		return nil
	}

	result := make(map[string]any, len(s.Fields))
	for key, val := range s.Fields {
		result[key] = v.convertQdrantValue(val)
	}

	return result
}

func (v *VectorStore) convertQdrantList(l *qdrant.ListValue) []any {
	if l == nil || len(l.Values) == 0 {
		return nil
	}

	result := make([]any, len(l.Values))
	for i, val := range l.Values {
		result[i] = v.convertQdrantValue(val)
	}

	return result
}

func (v *VectorStore) convertPayloadToMetadata(payload map[string]*qdrant.Value) map[string]any {
	if payload == nil {
		return nil
	}

	metadata := make(map[string]any, len(payload))
	for key, value := range payload {
		if value == nil {
			continue
		}
		metadata[key] = v.convertQdrantValue(value)
	}

	return metadata
}

func (v *VectorStore) buildDocumentsFromPoints(scoredPoints []*qdrant.ScoredPoint) ([]*document.Document, error) {
	docs := make([]*document.Document, 0, len(scoredPoints))

	for _, point := range scoredPoints {
		doc := &document.Document{}

		if pointID := point.GetId(); pointID != nil {
			doc.ID = pointID.GetUuid()
		}

		doc.Score = float64(point.GetScore())

		payload := point.GetPayload()
		if payload != nil {
			if contentValue, ok := payload[payloadDocumentContentKey]; ok {
				doc.Text = contentValue.GetStringValue()
			}

			delete(payload, payloadDocumentContentKey)

			doc.Metadata = v.convertPayloadToMetadata(payload)
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (v *VectorStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("qdrant: invalid retrieval request: %w", err)
	}

	queryPoints, err := v.buildQueryPoints(ctx, req)
	if err != nil {
		return nil, err
	}

	scoredPoints, err := v.client.Query(ctx, queryPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to query collection %s: %w", v.collectionName, err)
	}

	docs, err := v.buildDocumentsFromPoints(scoredPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to build documents from query results: %w", err)
	}

	return docs, nil
}

func (v *VectorStore) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("qdrant: invalid delete request: %w", err)
	}

	filter, err := ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("qdrant: failed to convert filter: %w", err)
	}

	_, err = v.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: v.collectionName,
		Points:         qdrant.NewPointsSelectorFilter(filter),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to delete points from collection %s: %w", v.collectionName, err)
	}

	return nil
}

func (v *VectorStore) Info() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: v.client,
		Provider:     Provider,
	}
}

func (v *VectorStore) Close() error {
	return v.client.Close()
}
