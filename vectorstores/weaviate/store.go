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
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
	"github.com/Tangerg/lynx/vectorstores/internal/batching"
	"github.com/Tangerg/lynx/vectorstores/internal/docio"
	"github.com/Tangerg/lynx/vectorstores/internal/scores"
	vectorconv "github.com/Tangerg/lynx/vectorstores/internal/vector"
)

const (
	Provider = "Weaviate"
)

const (
	fieldContent  = "content"
	fieldMetadata = "metadata"

	additionalID       = "id"
	additionalDistance = "distance"
)

// DistanceMetric selects the distance function configured on the Weaviate
// collection.
type DistanceMetric string

const (
	DistanceCosine    DistanceMetric = "cosine"
	DistanceDot       DistanceMetric = "dot"
	DistanceL2Squared DistanceMetric = "l2-squared"
	DistanceHamming   DistanceMetric = "hamming"
	DistanceManhattan DistanceMetric = "manhattan"
)

// StoreConfig contains configuration options for Weaviate vector store.
type StoreConfig struct {
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
	DocumentBatcher vectorstore.Batcher

	// DistanceMetric is the distance metric used for the HNSW vector index.
	// Valid values: "cosine" (default), "dot", "l2-squared", "hamming", "manhattan".
	// Optional: defaults to "cosine".
	DistanceMetric DistanceMetric
}

func (c StoreConfig) Validate() error {
	c.applyDefaults()
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
	switch c.DistanceMetric {
	case DistanceCosine, DistanceDot, DistanceL2Squared, DistanceHamming, DistanceManhattan:
	default:
		return fmt.Errorf("weaviate: unsupported DistanceMetric %q", c.DistanceMetric)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) applyDefaults() {
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

type Store struct {
	client           *weaviate.Client
	embeddingClient  *embeddingclient.Client
	documentBatcher  vectorstore.Batcher
	className        string
	distanceMetric   DistanceMetric
	initializeSchema bool
}

func NewStore(ctx context.Context, config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("weaviate: create embedding client: %w", err)
	}

	store := &Store{
		client:           config.Client,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		className:        config.ClassName,
		distanceMetric:   config.DistanceMetric,
		initializeSchema: config.InitializeSchema,
	}

	if err = store.initialize(ctx); err != nil {
		return nil, fmt.Errorf("weaviate: initialize vector store: %w", err)
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
		return fmt.Errorf("weaviate: check class existence: %w", err)
	}
	if exists {
		return nil
	}

	class := &models.Class{
		Class:           s.className,
		Vectorizer:      "none",
		VectorIndexType: "hnsw",
		VectorIndexConfig: map[string]any{
			"distance": string(s.distanceMetric),
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
		return fmt.Errorf("weaviate: create class %s: %w", s.className, err)
	}

	return nil
}

func (s *Store) buildObjects(docs []*document.Document, vectors [][]float64) ([]*models.Object, error) {
	objects := make([]*models.Object, 0, len(docs))

	for i, doc := range docs {
		metaBytes, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("weaviate: marshal metadata for document %s: %w", doc.ID, err)
		}

		obj := &models.Object{
			Class:  s.className,
			ID:     strfmt.UUID(doc.ID),
			Vector: models.C11yVector(vectorconv.Float32(vectors[i])),
			Properties: map[string]any{
				fieldContent:  doc.Text,
				fieldMetadata: string(metaBytes),
			},
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if err := docio.ValidateDocuments(docs); err != nil {
		return fmt.Errorf("weaviate.Store.Add: %w", err)
	}
	for i, doc := range docs {
		if err := validateObjectID(doc.ID); err != nil {
			return fmt.Errorf("weaviate.Store.Add: documents[%d]: %w", i, err)
		}
	}

	var batchedDocs [][]*document.Document
	batchedDocs, err = batching.Batch(ctx, s.documentBatcher, docs)
	if err != nil {
		return fmt.Errorf("weaviate: batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("weaviate: embed documents: %w", err)
		}

		objects, err := s.buildObjects(docs, vectors)
		if err != nil {
			return err
		}

		responses, err := s.client.Batch().ObjectsBatcher().
			WithObjects(objects...).
			Do(ctx)
		if err != nil {
			return fmt.Errorf("weaviate: batch insert %d objects to class %s: %w",
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
		WithVector(models.C11yVector(vectorconv.Float32(vector)))

	// WithCertainty is the minimum similarity threshold, only valid for cosine distance.
	if minScore > 0 && s.distanceMetric == DistanceCosine {
		builder = builder.WithCertainty(float32(minScore))
	}

	return builder
}

func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("weaviate.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("weaviate: embed query: %w", err)
	}

	fields := []graphql.Field{
		{Name: fieldContent},
		{Name: fieldMetadata},
		{
			Name: "_additional",
			Fields: []graphql.Field{
				{Name: additionalID},
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
			return nil, fmt.Errorf("weaviate: convert filter: %w", filterErr)
		}
		getBuilder = getBuilder.WithWhere(whereFilter)
	}

	result, err := getBuilder.Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("weaviate: query class %s: %w", s.className, err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("weaviate: GraphQL query error: %v", result.Errors[0].Message)
	}

	docs, err = s.buildDocumentsFromResult(result, req.MinScore)
	if err != nil {
		return nil, fmt.Errorf("weaviate: build documents from results: %w", err)
	}

	return docs, nil
}

func (s *Store) buildDocumentsFromResult(
	result *models.GraphQLResponse,
	minScore float64,
) ([]vectorstore.Match, error) {
	if result == nil {
		return nil, errors.New("weaviate: GraphQL response is nil")
	}
	getData, ok := result.Data["Get"]
	if !ok {
		return nil, errors.New("weaviate: GraphQL response is missing Get data")
	}

	getMap, ok := getData.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("weaviate: GraphQL Get data has type %T, want object", getData)
	}

	classData, ok := getMap[s.className]
	if !ok {
		return nil, fmt.Errorf("weaviate: GraphQL Get data is missing class %q", s.className)
	}

	items, ok := classData.([]any)
	if !ok {
		return nil, fmt.Errorf("weaviate: GraphQL class data has type %T, want array", classData)
	}

	docs := make([]vectorstore.Match, 0, len(items))

	for _, item := range items {
		objMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("weaviate: result object has type %T, want object", item)
		}

		doc := &document.Document{}
		additional, ok := objMap["_additional"].(map[string]any)
		if !ok {
			return nil, errors.New("weaviate: result object is missing _additional")
		}
		id, ok := additional[additionalID].(string)
		if !ok || id == "" {
			return nil, errors.New("weaviate: result object is missing _additional.id")
		}
		doc.ID = id
		distance, ok := additional[additionalDistance].(float64)
		if !ok {
			return nil, fmt.Errorf("weaviate: result distance has type %T, want number", additional[additionalDistance])
		}
		score := s.normalizeDistance(distance)
		if score < minScore {
			continue
		}

		content, ok := objMap[fieldContent].(string)
		if !ok || content == "" {
			return nil, fmt.Errorf("weaviate: result object is missing %s", fieldContent)
		}
		doc.Text = content

		if metaStr, ok := objMap[fieldMetadata].(string); ok && metaStr != "" && metaStr != "null" {
			if err := json.Unmarshal([]byte(metaStr), &doc.Metadata); err != nil {
				return nil, fmt.Errorf("weaviate: decode metadata: %w", err)
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}

	return docs, nil
}

func (s *Store) normalizeDistance(distance float64) float64 {
	switch s.distanceMetric {
	case DistanceCosine:
		return scores.CosineDistance(distance)
	case DistanceDot:
		return scores.NegativeInnerProductDistance(distance)
	case DistanceL2Squared, DistanceHamming, DistanceManhattan:
		return scores.Distance(distance)
	default:
		return scores.Bounded(distance)
	}
}

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("weaviate.Store.DeleteWhere: %w", err)
	}

	var whereFilter *filters.WhereBuilder
	whereFilter, err = ToFilter(expr)
	if err != nil {
		return fmt.Errorf("weaviate: convert filter: %w", err)
	}

	_, err = s.client.Batch().ObjectsBatchDeleter().
		WithClassName(s.className).
		WithWhere(whereFilter).
		Do(ctx)
	if err != nil {
		return fmt.Errorf("weaviate: delete from class %s: %w", s.className, err)
	}

	return nil
}

// DeleteIDs removes objects by their Weaviate UUIDs. An empty slice is a
// no-op; unknown ids are ignored (idempotent).
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}
	for i, id := range ids {
		if err := validateObjectID(id); err != nil {
			return fmt.Errorf("weaviate.Store.DeleteIDs: ids[%d]: %w", i, err)
		}
	}

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
			err = fmt.Errorf("weaviate: delete object %s from class %s: %w",
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

func validateObjectID(id string) error {
	if err := uuid.Validate(id); err != nil {
		return fmt.Errorf("%w %q: must be a UUID", ErrInvalidObjectID, id)
	}
	return nil
}
