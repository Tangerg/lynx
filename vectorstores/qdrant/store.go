package qdrant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const (
	Provider = "Qdrant"
)

const (
	// payloadDocumentContentKey is the payload key for saving document content
	payloadDocumentContentKey = "lynx:ai:vectorstore:qdrant:payload_document_content"
)

// StoreConfig contains configuration options for Qdrant vector store.
type StoreConfig struct {
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
	DocumentBatcher vectorstores.Batcher

	// StoreDocumentContent determines whether to store the original document
	// content in the payload. When true, the full text will be saved with a
	// special key, allowing retrieval of original content without external storage.
	// Optional: defaults to false to save storage space.
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
// [context.Background].
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
}

var (
	_ vectorstore.Store     = (*Store)(nil)
	_ vectorstore.IDDeleter = (*Store)(nil)
)

type Store struct {
	client               *qdrant.Client
	embeddingModel       embedding.Model
	embeddingClient      *embedding.Client
	documentBatcher      vectorstores.Batcher
	collectionName       string
	initializeSchema     bool
	storeDocumentContent bool
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:               config.Client,
		embeddingModel:       config.EmbeddingModel,
		embeddingClient:      embeddingClient,
		documentBatcher:      config.DocumentBatcher,
		collectionName:       config.CollectionName,
		initializeSchema:     config.InitializeSchema,
		storeDocumentContent: config.StoreDocumentContent,
	}

	if err = store.initialize(config.Context); err != nil {
		return nil, fmt.Errorf("qdrant: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	if !s.initializeSchema {
		return nil
	}

	exists, err := s.client.CollectionExists(ctx, s.collectionName)
	if err != nil {
		return fmt.Errorf("qdrant: failed to check collection existence: %w", err)
	}

	if exists {
		return nil
	}

	dimensions := s.embeddingModel.Dimensions(ctx)
	if dimensions <= 0 {
		return errors.New("qdrant: dimensions must be greater than zero")
	}

	err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: s.collectionName,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     uint64(dimensions),
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to create collection %s: %w", s.collectionName, err)
	}

	return nil
}

func (s *Store) buildUpsertPoints(ctx context.Context, req *vectorstore.CreateRequest) (*qdrant.UpsertPoints, error) {
	upsertPoints := &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Wait:           new(true),
	}

	batchedDocs, err := s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.
			embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return nil, fmt.Errorf("qdrant: failed to generate vectors: %w", err)
		}

		for i, doc := range docs {
			point, err := s.buildPointStruct(doc, vectors[i])
			if err != nil {
				return nil, fmt.Errorf("qdrant: failed to build point for document %s: %w", doc.ID, err)
			}

			upsertPoints.Points = append(upsertPoints.Points, point)
		}
	}

	return upsertPoints, nil
}

func (s *Store) buildPointStruct(doc *document.Document, vector []float64) (*qdrant.PointStruct, error) {
	id := uuid.NewString()

	point := &qdrant.PointStruct{
		Id:      qdrant.NewID(id),
		Vectors: qdrant.NewVectors(math.ConvertSlice[float64, float32](vector)...),
	}

	metadataValues, err := doc.Metadata.Values()
	if err != nil {
		return nil, fmt.Errorf("qdrant: decode metadata: %w", err)
	}
	payload, err := qdrant.TryValueMap(metadataValues)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to convert metadata to payload: %w", err)
	}
	point.Payload = payload

	if s.storeDocumentContent {
		contentValue, err := qdrant.NewValue(doc.Text)
		if err != nil {
			return nil, fmt.Errorf("qdrant: failed to create content value: %w", err)
		}
		point.Payload[payloadDocumentContentKey] = contentValue
	}

	return point, nil
}

func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("qdrant: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "qdrant", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var upsertPoints *qdrant.UpsertPoints
	upsertPoints, err = s.buildUpsertPoints(ctx, req)
	if err != nil {
		return err
	}

	_, err = s.client.Upsert(ctx, upsertPoints)
	if err != nil {
		return fmt.Errorf("qdrant: failed to upsert %d points to collection %s: %w",
			len(upsertPoints.Points), s.collectionName, err)
	}

	return nil
}

func (s *Store) buildQueryPoints(ctx context.Context, req *vectorstore.RetrievalRequest) (*qdrant.QueryPoints, error) {
	queryPoints := &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		ScoreThreshold: new(float32(req.MinScore)),
		Limit:          new(uint64(req.TopK)),
		WithPayload:    qdrant.NewWithPayload(true),
	}

	if req.Filter != nil {
		filter, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("qdrant: failed to convert filter: %w", err)
		}
		queryPoints.Filter = filter
	}

	vector, _, err := s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to embed query text: %w", err)
	}

	queryPoints.Query = qdrant.NewQuery(math.ConvertSlice[float64, float32](vector)...)

	return queryPoints, nil
}

func (s *Store) convertQdrantValue(value *qdrant.Value) any {
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
		return s.convertQdrantStruct(kind.StructValue)
	case *qdrant.Value_ListValue:
		return s.convertQdrantList(kind.ListValue)
	default:
		return nil
	}
}

func (s *Store) convertQdrantStruct(qs *qdrant.Struct) map[string]any {
	if qs == nil || qs.Fields == nil {
		return nil
	}

	result := make(map[string]any, len(qs.Fields))
	for key, val := range qs.Fields {
		result[key] = s.convertQdrantValue(val)
	}

	return result
}

func (s *Store) convertQdrantList(l *qdrant.ListValue) []any {
	if l == nil || len(l.Values) == 0 {
		return nil
	}

	result := make([]any, len(l.Values))
	for i, val := range l.Values {
		result[i] = s.convertQdrantValue(val)
	}

	return result
}

func (s *Store) convertPayloadToMetadata(payload map[string]*qdrant.Value) map[string]any {
	if payload == nil {
		return nil
	}

	metadata := make(map[string]any, len(payload))
	for key, value := range payload {
		if value == nil {
			continue
		}
		metadata[key] = s.convertQdrantValue(value)
	}

	return metadata
}

func (s *Store) buildDocumentsFromPoints(scoredPoints []*qdrant.ScoredPoint) ([]vectorstore.Match, error) {
	docs := make([]vectorstore.Match, 0, len(scoredPoints))

	for _, point := range scoredPoints {
		doc := &document.Document{}

		if pointID := point.GetId(); pointID != nil {
			doc.ID = pointID.GetUuid()
		}

		payload := point.GetPayload()
		if payload != nil {
			if contentValue, ok := payload[payloadDocumentContentKey]; ok {
				doc.Text = contentValue.GetStringValue()
			}

			delete(payload, payloadDocumentContentKey)

			var err error
			doc.Metadata, err = metadata.FromValues(s.convertPayloadToMetadata(payload))
			if err != nil {
				return nil, fmt.Errorf("qdrant: encode metadata: %w", err)
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: float64(point.GetScore())})
	}

	return docs, nil
}

func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("qdrant: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "qdrant", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var queryPoints *qdrant.QueryPoints
	queryPoints, err = s.buildQueryPoints(ctx, req)
	if err != nil {
		return nil, err
	}

	var scoredPoints []*qdrant.ScoredPoint
	scoredPoints, err = s.client.Query(ctx, queryPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to query collection %s: %w", s.collectionName, err)
	}

	docs, err = s.buildDocumentsFromPoints(scoredPoints)
	if err != nil {
		return nil, fmt.Errorf("qdrant: failed to build documents from query results: %w", err)
	}

	return docs, nil
}

func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("qdrant: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "qdrant")
	defer func() { tracing.Finish(span, err) }()

	var filter *qdrant.Filter
	filter, err = ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("qdrant: failed to convert filter: %w", err)
	}

	_, err = s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Points:         qdrant.NewPointsSelectorFilter(filter),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to delete points from collection %s: %w", s.collectionName, err)
	}

	return nil
}

// DeleteByIDs removes points by their Qdrant point ids. Each id is the
// UUID surfaced as document.ID by Retrieve (buildPointStruct assigns a
// fresh UUID via qdrant.NewID, and buildDocumentsFromPoints reads it back
// out), so the same qdrant.NewID conversion maps an id back to a *PointId.
// An empty slice is a no-op; unknown ids are silently ignored (idempotent).
// Implements [vectorstore.IDDeleter].
func (s *Store) DeleteByIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "qdrant")
	defer func() { tracing.Finish(span, err) }()

	pointIDs := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		pointIDs[i] = qdrant.NewID(id)
	}

	_, err = s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Points:         qdrant.NewPointsSelector(pointIDs...),
	})
	if err != nil {
		return fmt.Errorf("qdrant: failed to delete points by ids from collection %s: %w", s.collectionName, err)
	}

	return nil
}

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.client,
		Provider:     Provider,
	}
}

func (s *Store) Close() error {
	return s.client.Close()
}
