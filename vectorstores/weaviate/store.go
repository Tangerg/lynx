package weaviate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
)

const (
	Provider = "Weaviate"
)

const (
	fieldContent  = "content"
	fieldMetadata = "metadata"

	additionalID        = "id"
	additionalCertainty = "certainty"
	additionalDistance  = "distance"
)

// VectorStoreConfig contains configuration options for Weaviate vector store.
type VectorStoreConfig struct {
	// Context is the context for all operations.
	// Optional: defaults to context.Background() if nil.
	Context context.Context

	// Client is the Weaviate client instance.
	// Required: must be provided, otherwise initialization will fail.
	Client *weaviate.Client

	// ClassName is the name of the Weaviate class (collection) to use.
	// Required: must be a non-empty string.
	ClassName string

	// InitializeSchema indicates whether to automatically create the class
	// if it does not exist. When set to true, the class will be created
	// with HNSW vector index configuration based on the chosen DistanceMetric.
	// Optional: defaults to false.
	InitializeSchema bool

	// EmbeddingModel is the model used to generate vector embeddings from text.
	// Required: must be provided for both embedding generation and schema initialization.
	EmbeddingModel embedding.Model

	// DocumentBatcher is responsible for batching documents before insertion.
	// Required: must be provided to handle document batching logic.
	DocumentBatcher document.Batcher

	// StoreDocumentContent determines whether to store the original document
	// content in the content field.
	// Optional: defaults to false.
	StoreDocumentContent bool

	// DistanceMetric is the distance metric used for the HNSW vector index.
	// Valid values: "cosine" (default), "dot", "l2-squared", "hamming", "manhattan".
	// Optional: defaults to "cosine".
	DistanceMetric string
}

func (c *VectorStoreConfig) validate() error {
	if c == nil {
		return errors.New("weaviate: config is nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Client == nil {
		return errors.New("weaviate: client is required")
	}
	if c.ClassName == "" {
		return errors.New("weaviate: class name is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("weaviate: embedding model is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("weaviate: document batcher is required")
	}
	if c.DistanceMetric == "" {
		c.DistanceMetric = "cosine"
	}
	return nil
}

var _ vectorstore.VectorStore = (*VectorStore)(nil)

type VectorStore struct {
	client               *weaviate.Client
	embeddingModel       embedding.Model
	embeddingClient      *embedding.Client
	documentBatcher      document.Batcher
	className            string
	distanceMetric       string
	initializeSchema     bool
	storeDocumentContent bool
}

func NewVectorStore(config *VectorStoreConfig) (*VectorStore, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClientWithModel(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to create embedding client: %w", err)
	}

	store := &VectorStore{
		client:               config.Client,
		embeddingModel:       config.EmbeddingModel,
		embeddingClient:      embeddingClient,
		documentBatcher:      config.DocumentBatcher,
		className:            config.ClassName,
		distanceMetric:       config.DistanceMetric,
		initializeSchema:     config.InitializeSchema,
		storeDocumentContent: config.StoreDocumentContent,
	}

	if err = store.initialize(config.Context); err != nil {
		return nil, fmt.Errorf("weaviate: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (v *VectorStore) initialize(ctx context.Context) error {
	if !v.initializeSchema {
		return nil
	}

	exists, err := v.client.Schema().ClassExistenceChecker().
		WithClassName(v.className).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to check class existence: %w", err)
	}
	if exists {
		return nil
	}

	class := &models.Class{
		Class:           v.className,
		Vectorizer:      "none",
		VectorIndexType: "hnsw",
		VectorIndexConfig: map[string]interface{}{
			"distance": v.distanceMetric,
		},
		Properties: []*models.Property{
			{
				Name:     fieldContent,
				DataType: []string{"text"},
			},
			{
				Name:     fieldMetadata,
				DataType: []string{"text"},
			},
		},
	}

	if err = v.client.Schema().ClassCreator().WithClass(class).Do(ctx); err != nil {
		return fmt.Errorf("weaviate: failed to create class %s: %w", v.className, err)
	}

	return nil
}

func (v *VectorStore) buildObjects(docs []*document.Document, vectors [][]float64) ([]*models.Object, error) {
	objects := make([]*models.Object, 0, len(docs))

	for i, doc := range docs {
		content := ""
		if v.storeDocumentContent {
			content = doc.Text
		}

		metaBytes, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("weaviate: failed to marshal metadata for document %s: %w", doc.ID, err)
		}

		obj := &models.Object{
			Class:  v.className,
			ID:     strfmt.UUID(uuid.NewString()),
			Vector: models.C11yVector(math.ConvertSlice[float64, float32](vectors[i])),
			Properties: map[string]interface{}{
				fieldContent:  content,
				fieldMetadata: string(metaBytes),
			},
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func (v *VectorStore) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("weaviate: invalid create request: %w", err)
	}

	batchedDocs, err := v.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("weaviate: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := v.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: failed to generate vectors: %w", err)
		}

		objects, err := v.buildObjects(docs, vectors)
		if err != nil {
			return err
		}

		responses, err := v.client.Batch().ObjectsBatcher().
			WithObjects(objects...).
			Do(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: failed to batch insert %d objects to class %s: %w",
				len(objects), v.className, err)
		}

		for j := range responses {
			resp := &responses[j]
			if resp.Result != nil && resp.Result.Errors != nil {
				return fmt.Errorf("weaviate: batch insert error for object %s: %v",
					resp.ID, resp.Result.Errors.Error)
			}
		}
	}

	return nil
}

func (v *VectorStore) buildNearVector(vector []float64, minScore float64) *graphql.NearVectorArgumentBuilder {
	builder := v.client.GraphQL().NearVectorArgBuilder().
		WithVector(models.C11yVector(math.ConvertSlice[float64, float32](vector)))

	// WithCertainty is the minimum similarity threshold, only valid for cosine distance.
	if minScore > 0 && v.distanceMetric == "cosine" {
		builder = builder.WithCertainty(float32(minScore))
	}

	return builder
}

func (v *VectorStore) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("weaviate: invalid retrieval request: %w", err)
	}

	vector, _, err := v.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to embed query text: %w", err)
	}

	fields := []graphql.Field{
		{Name: fieldContent},
		{Name: fieldMetadata},
		{
			Name: "_additional",
			Fields: []graphql.Field{
				{Name: additionalID},
				{Name: additionalCertainty},
				{Name: additionalDistance},
			},
		},
	}

	getBuilder := v.client.GraphQL().Get().
		WithClassName(v.className).
		WithFields(fields...).
		WithNearVector(v.buildNearVector(vector, req.MinScore)).
		WithLimit(req.TopK)

	if req.Filter != nil {
		whereFilter, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("weaviate: failed to convert filter: %w", err)
		}
		getBuilder = getBuilder.WithWhere(whereFilter)
	}

	result, err := getBuilder.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to query class %s: %w", v.className, err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("weaviate: GraphQL query error: %v", result.Errors[0].Message)
	}

	docs, err := v.buildDocumentsFromResult(result)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to build documents from results: %w", err)
	}

	return docs, nil
}

func (v *VectorStore) buildDocumentsFromResult(result *models.GraphQLResponse) ([]*document.Document, error) {
	getData, ok := result.Data["Get"]
	if !ok {
		return nil, nil
	}

	getMap, ok := getData.(map[string]interface{})
	if !ok {
		return nil, nil
	}

	classData, ok := getMap[v.className]
	if !ok {
		return nil, nil
	}

	items, ok := classData.([]interface{})
	if !ok {
		return nil, nil
	}

	docs := make([]*document.Document, 0, len(items))

	for _, item := range items {
		objMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		doc := &document.Document{}

		if additional, ok := objMap["_additional"].(map[string]interface{}); ok {
			if id, ok := additional[additionalID].(string); ok {
				doc.ID = id
			}
			if certainty, ok := additional[additionalCertainty].(float64); ok {
				doc.Score = certainty
			} else if distance, ok := additional[additionalDistance].(float64); ok {
				// Convert distance to a similarity score: smaller distance = higher score.
				doc.Score = 1.0 - distance
			}
		}

		if v.storeDocumentContent {
			if content, ok := objMap[fieldContent].(string); ok {
				doc.Text = content
			}
		}

		if metaStr, ok := objMap[fieldMetadata].(string); ok && metaStr != "" && metaStr != "null" {
			var metadata map[string]any
			if err := json.Unmarshal([]byte(metaStr), &metadata); err == nil {
				doc.Metadata = metadata
			}
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (v *VectorStore) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("weaviate: invalid delete request: %w", err)
	}

	whereFilter, err := ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("weaviate: failed to convert filter: %w", err)
	}

	_, err = v.client.Batch().ObjectsBatchDeleter().
		WithClassName(v.className).
		WithWhere(whereFilter).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to delete from class %s: %w", v.className, err)
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
	// Weaviate HTTP client does not require explicit closing.
	return nil
}
