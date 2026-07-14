package weaviate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/weaviate/weaviate-go-client/v5/weaviate"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/fault"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"
	"github.com/weaviate/weaviate-go-client/v5/weaviate/graphql"
	"github.com/weaviate/weaviate/entities/models"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
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

// StoreConfig contains configuration options for Weaviate vector store.
type StoreConfig struct {
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
	DocumentBatcher vectorstores.Batcher

	// StoreDocumentContent determines whether to store the original document
	// content in the content field.
	// Optional: defaults to false.
	StoreDocumentContent bool

	// DistanceMetric is the distance metric used for the HNSW vector index.
	// Valid values: "cosine" (default), "dot", "l2-squared", "hamming", "manhattan".
	// Optional: defaults to "cosine".
	DistanceMetric string
}

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return ErrMissingClient
	}
	if c.ClassName == "" {
		return ErrMissingClassName
	}
	if c.EmbeddingModel == nil {
		return ErrMissingEmbeddingModel
	}
	if c.DocumentBatcher == nil {
		return ErrMissingDocumentBatcher
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.DistanceMetric == "" {
		c.DistanceMetric = "cosine"
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

type Store struct {
	client               *weaviate.Client
	embeddingClient      *embedding.Client
	documentBatcher      vectorstores.Batcher
	className            string
	distanceMetric       string
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
		return nil, fmt.Errorf("weaviate: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:               config.Client,
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

func (s *Store) initialize(ctx context.Context) error {
	if !s.initializeSchema {
		return nil
	}

	exists, err := s.client.Schema().ClassExistenceChecker().
		WithClassName(s.className).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to check class existence: %w", err)
	}
	if exists {
		return nil
	}

	class := &models.Class{
		Class:           s.className,
		Vectorizer:      "none",
		VectorIndexType: "hnsw",
		VectorIndexConfig: map[string]any{
			"distance": s.distanceMetric,
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

	if err = s.client.Schema().ClassCreator().WithClass(class).Do(ctx); err != nil {
		return fmt.Errorf("weaviate: failed to create class %s: %w", s.className, err)
	}

	return nil
}

func (s *Store) buildObjects(docs []*document.Document, vectors [][]float64) ([]*models.Object, error) {
	objects := make([]*models.Object, 0, len(docs))

	for i, doc := range docs {
		content := ""
		if s.storeDocumentContent {
			content = doc.Text
		}

		metaBytes, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("weaviate: failed to marshal metadata for document %s: %w", doc.ID, err)
		}

		obj := &models.Object{
			Class:  s.className,
			ID:     strfmt.UUID(uuid.NewString()),
			Vector: models.C11yVector(math.ConvertSlice[float64, float32](vectors[i])),
			Properties: map[string]any{
				fieldContent:  content,
				fieldMetadata: string(metaBytes),
			},
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "weaviate", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("weaviate: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: failed to generate vectors: %w", err)
		}

		objects, err := s.buildObjects(docs, vectors)
		if err != nil {
			return err
		}

		responses, err := s.client.Batch().ObjectsBatcher().
			WithObjects(objects...).
			Do(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: failed to batch insert %d objects to class %s: %w",
				len(objects), s.className, err)
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

func (s *Store) buildNearVector(vector []float64, minScore float64) *graphql.NearVectorArgumentBuilder {
	builder := s.client.GraphQL().NearVectorArgBuilder().
		WithVector(models.C11yVector(math.ConvertSlice[float64, float32](vector)))

	// WithCertainty is the minimum similarity threshold, only valid for cosine distance.
	if minScore > 0 && s.distanceMetric == "cosine" {
		builder = builder.WithCertainty(float32(minScore))
	}

	return builder
}

func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("weaviate: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "weaviate", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
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

	getBuilder := s.client.GraphQL().Get().
		WithClassName(s.className).
		WithFields(fields...).
		WithNearVector(s.buildNearVector(vector, req.MinScore)).
		WithLimit(req.TopK)

	if req.Filter != nil {
		whereFilter, filterErr := ToFilter(req.Filter)
		if filterErr != nil {
			return nil, fmt.Errorf("weaviate: failed to convert filter: %w", filterErr)
		}
		getBuilder = getBuilder.WithWhere(whereFilter)
	}

	result, err := getBuilder.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to query class %s: %w", s.className, err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("weaviate: GraphQL query error: %v", result.Errors[0].Message)
	}

	docs, err = s.buildDocumentsFromResult(result)
	if err != nil {
		return nil, fmt.Errorf("weaviate: failed to build documents from results: %w", err)
	}

	return docs, nil
}

func (s *Store) buildDocumentsFromResult(result *models.GraphQLResponse) ([]vectorstore.Match, error) {
	getData, ok := result.Data["Get"]
	if !ok {
		return nil, nil
	}

	getMap, ok := getData.(map[string]any)
	if !ok {
		return nil, nil
	}

	classData, ok := getMap[s.className]
	if !ok {
		return nil, nil
	}

	items, ok := classData.([]any)
	if !ok {
		return nil, nil
	}

	docs := make([]vectorstore.Match, 0, len(items))

	for _, item := range items {
		objMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		doc := &document.Document{}
		var score float64

		if additional, ok := objMap["_additional"].(map[string]any); ok {
			if id, ok := additional[additionalID].(string); ok {
				doc.ID = id
			}
			if certainty, ok := additional[additionalCertainty].(float64); ok {
				score = certainty
			} else if distance, ok := additional[additionalDistance].(float64); ok {
				// Convert distance to a similarity score: smaller distance = higher score.
				score = 1.0 - distance
			}
		}

		if s.storeDocumentContent {
			if content, ok := objMap[fieldContent].(string); ok {
				doc.Text = content
			}
		}

		if metaStr, ok := objMap[fieldMetadata].(string); ok && metaStr != "" && metaStr != "null" {
			if err := json.Unmarshal([]byte(metaStr), &doc.Metadata); err != nil {
				return nil, fmt.Errorf("weaviate: decode metadata: %w", err)
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}

	return docs, nil
}

func (s *Store) DeleteWhere(ctx context.Context, expr ast.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Analyze(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "weaviate")
	defer func() { tracing.Finish(span, err) }()

	var whereFilter *filters.WhereBuilder
	whereFilter, err = ToFilter(expr)
	if err != nil {
		return fmt.Errorf("weaviate: failed to convert filter: %w", err)
	}

	_, err = s.client.Batch().ObjectsBatchDeleter().
		WithClassName(s.className).
		WithWhere(whereFilter).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: failed to delete from class %s: %w", s.className, err)
	}

	return nil
}

// DeleteIDs removes objects by their Weaviate object UUID. The ids are
// the same identifiers surfaced as document.ID by Retrieve (the object's
// `_additional.id`), since Create assigns each object a UUID that becomes
// its primary key. An empty slice is a no-op; unknown ids are silently
// ignored (Weaviate's per-object Deleter is idempotent). Implements
// [vectorstore.IDDeleter].
//
// One `db.vector.delete weaviate` span per call.
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "weaviate")
	defer func() { tracing.Finish(span, err) }()

	for _, id := range ids {
		if delErr := s.client.Data().Deleter().
			WithClassName(s.className).
			WithID(id).
			Do(ctx); delErr != nil {
			// A missing object yields a 404; treat unknown ids as a no-op
			// so the operation stays idempotent.
			if clientErr, ok := errors.AsType[*fault.WeaviateClientError](delErr); ok && clientErr.StatusCode == http.StatusNotFound {
				continue
			}
			err = fmt.Errorf("weaviate: failed to delete object %s from class %s: %w",
				id, s.className, delErr)
			return err
		}
	}

	return nil
}

func (s *Store) Close() error {
	// Weaviate HTTP client does not require explicit closing.
	return nil
}
