package azurecosmos

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/ident"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "AzureCosmosDB"

const (
	DefaultIDField        = "id"
	DefaultContentField   = "content"
	DefaultMetadataField  = "metadata"
	DefaultEmbeddingField = "embedding"
	DefaultPartitionKey   = "/id"
	docAlias              = "c"
)

// DistanceFunction names the function passed to VectorDistance().
// The chosen value must match the container's vector embedding
// policy.
type DistanceFunction string

const (
	DistanceCosine     DistanceFunction = "cosine"
	DistanceDotProduct DistanceFunction = "dotproduct"
	DistanceEuclidean  DistanceFunction = "euclidean"
)

// safeIdentifier matches the standard SQL unquoted identifier shape.

// StoreConfig contains configuration options for the Azure Cosmos DB
// NoSQL vector store.
type StoreConfig struct {
	// Context is unused at construction — Cosmos DB schemas are
	// managed out of band — but kept for forward compatibility.
	Context context.Context

	// Container is the Cosmos container that holds the documents.
	// The caller is responsible for provisioning it with the right
	// vector embedding policy + indexing policy (set up in Azure
	// Portal / ARM / Terraform). Required.
	Container *azcosmos.ContainerClient

	// PartitionKeyPath is the container's partition-key path,
	// recorded so the store can compute partition keys for upsert
	// and delete. Optional: defaults to [DefaultPartitionKey] ("/id").
	PartitionKeyPath string

	// IDField / ContentField / MetadataField / EmbeddingField
	// override the JSON property names on the stored documents.
	IDField        string
	ContentField   string
	MetadataField  string
	EmbeddingField string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upsert. Required.
	DocumentBatcher document.Batcher

	// DistanceFunction selects the function passed to
	// VectorDistance(). Optional: defaults to [DistanceCosine].
	DistanceFunction DistanceFunction
}

func (c StoreConfig) Validate() error {
	if c.Container == nil {
		return errors.New("azurecosmos: Container is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("azurecosmos: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("azurecosmos: DocumentBatcher is required")
	}
	return ident.Check("azurecosmos", map[string]string{
		"IDField":        c.IDField,
		"ContentField":   c.ContentField,
		"MetadataField":  c.MetadataField,
		"EmbeddingField": c.EmbeddingField,
	})
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.IDField = cmp.Or(c.IDField, DefaultIDField)
	c.ContentField = cmp.Or(c.ContentField, DefaultContentField)
	c.MetadataField = cmp.Or(c.MetadataField, DefaultMetadataField)
	c.EmbeddingField = cmp.Or(c.EmbeddingField, DefaultEmbeddingField)
	c.PartitionKeyPath = cmp.Or(c.PartitionKeyPath, DefaultPartitionKey)
	c.DistanceFunction = cmp.Or(c.DistanceFunction, DistanceCosine)
}

var _ vectorstore.Store = (*Store)(nil)

// Store is an Azure Cosmos DB NoSQL backed [vectorstore.Store]
// implementation. The container is expected to be provisioned with a
// vector embedding policy that matches [StoreConfig.DistanceFunction]
// and the embedding model's dimensionality.
type Store struct {
	container        *azcosmos.ContainerClient
	idField          string
	contentField     string
	metadataField    string
	embeddingField   string
	partitionKeyPath string
	embeddingModel   embedding.Model
	embeddingClient  *embedding.Client
	documentBatcher  document.Batcher
	distanceFunc     DistanceFunction
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("azurecosmos: failed to create embedding client: %w", err)
	}

	return &Store{
		container:        config.Container,
		idField:          config.IDField,
		contentField:     config.ContentField,
		metadataField:    config.MetadataField,
		embeddingField:   config.EmbeddingField,
		partitionKeyPath: config.PartitionKeyPath,
		embeddingModel:   config.EmbeddingModel,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		distanceFunc:     config.DistanceFunction,
	}, nil
}

// Create embeds documents and upserts them.
func (s *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("azurecosmos: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "azurecosmos", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("azurecosmos: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("azurecosmos: failed to generate embeddings: %w", err)
		}

		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
			payload := map[string]any{
				s.idField:        id,
				s.contentField:   doc.Text,
				s.metadataField:  metaOrEmpty(doc.Metadata),
				s.embeddingField: math.ConvertSlice[float64, float32](vectors[i]),
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("azurecosmos: marshal item %s: %w", id, err)
			}
			if _, err := s.container.UpsertItem(ctx, azcosmos.NewPartitionKeyString(id), body, nil); err != nil {
				return fmt.Errorf("azurecosmos: upsert %s: %w", id, err)
			}
		}
	}
	return nil
}

// Retrieve runs a VectorDistance-ordered query.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("azurecosmos: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "azurecosmos", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("azurecosmos: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	wherePredicate, params, err := s.buildFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	whereClause := ""
	if wherePredicate != "" {
		whereClause = " WHERE " + wherePredicate
	}

	distanceCall := fmt.Sprintf("VectorDistance(c.%s, @queryVec, false, {'distanceFunction':'%s'})",
		s.embeddingField, s.distanceFunc)

	query := fmt.Sprintf(
		"SELECT TOP @topK c.%s AS _id, c.%s AS _content, c.%s AS _metadata, %s AS _distance FROM c%s ORDER BY %s",
		s.idField, s.contentField, s.metadataField, distanceCall, whereClause, distanceCall,
	)

	queryParams := append([]azcosmos.QueryParameter(nil),
		azcosmos.QueryParameter{Name: "@queryVec", Value: queryVec},
		azcosmos.QueryParameter{Name: "@topK", Value: req.TopK},
	)
	for _, p := range params {
		queryParams = append(queryParams, azcosmos.QueryParameter{Name: p.Name, Value: p.Value})
	}

	// Cross-partition query: pass the canonical empty partition key.
	pager := s.container.NewQueryItemsPager(query, azcosmos.NewPartitionKey(), &azcosmos.QueryOptions{
		QueryParameters: queryParams,
	})

	docs = make([]*document.Document, 0, req.TopK)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("azurecosmos: query: %w", err)
		}
		for _, item := range page.Items {
			doc, err := s.decodeRow(item, req.MinScore)
			if err != nil {
				return nil, err
			}
			if doc != nil {
				docs = append(docs, doc)
			}
		}
	}
	return docs, nil
}

// Delete removes documents matching the filter expression.
func (s *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("azurecosmos: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "azurecosmos")
	defer func() { tracing.Finish(span, err) }()

	predicate, params, err := s.buildFilter(req.Filter)
	if err != nil {
		return err
	}
	if predicate == "" {
		return errors.New("azurecosmos: refusing to delete on empty filter")
	}

	query := fmt.Sprintf("SELECT c.%s AS _id FROM c WHERE %s", s.idField, predicate)
	queryParams := make([]azcosmos.QueryParameter, 0, len(params))
	for _, p := range params {
		queryParams = append(queryParams, azcosmos.QueryParameter{Name: p.Name, Value: p.Value})
	}

	pager := s.container.NewQueryItemsPager(query, azcosmos.NewPartitionKey(), &azcosmos.QueryOptions{
		QueryParameters: queryParams,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("azurecosmos: enumerate ids: %w", err)
		}
		for _, item := range page.Items {
			var holder struct {
				ID string `json:"_id"`
			}
			if err := json.Unmarshal(item, &holder); err != nil {
				return fmt.Errorf("azurecosmos: decode id: %w", err)
			}
			if _, err := s.container.DeleteItem(ctx, azcosmos.NewPartitionKeyString(holder.ID), holder.ID, nil); err != nil {
				return fmt.Errorf("azurecosmos: delete %s: %w", holder.ID, err)
			}
		}
	}
	return nil
}

func (s *Store) buildFilter(filter ast.Expr) (string, []NamedParam, error) {
	if filter == nil {
		return "", nil, nil
	}
	v := NewVisitor(docAlias, s.metadataField)
	v.Visit(filter)
	if err := v.Error(); err != nil {
		return "", nil, fmt.Errorf("azurecosmos: convert filter: %w", err)
	}
	predicate, params := v.Result()
	return predicate, params, nil
}

// decodeRow turns a Cosmos JSON row into a Document, applying the
// MinScore filter using the distance-to-score helper.
func (s *Store) decodeRow(raw json.RawMessage, minScore float64) (*document.Document, error) {
	var row struct {
		ID       string         `json:"_id"`
		Content  string         `json:"_content"`
		Metadata map[string]any `json:"_metadata"`
		Distance float64        `json:"_distance"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, fmt.Errorf("azurecosmos: decode row: %w", err)
	}

	score := s.distanceToScore(row.Distance)
	if score < minScore {
		return nil, nil
	}
	return &document.Document{
		ID:       row.ID,
		Text:     row.Content,
		Metadata: row.Metadata,
		Score:    score,
	}, nil
}

func (s *Store) distanceToScore(distance float64) float64 {
	switch s.distanceFunc {
	case DistanceEuclidean:
		return 1.0 / (1.0 + distance)
	case DistanceDotProduct:
		score := (1.0 + distance) / 2.0
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
		// Cosine via VectorDistance returns 1 - cosine_similarity,
		// range [0, 2]; (1 - d/2) collapses to [0, 1].
		score := 1.0 - distance/2.0
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

func (s *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: s.container,
		Provider:     Provider,
	}
}

func (s *Store) Close() error { return nil }

func metaOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
