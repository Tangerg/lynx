package milvus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const (
	Provider = "Milvus"
)

const (
	fieldID      = "id"
	fieldVector  = "vector"
	fieldContent = "content"
	fieldMeta    = "metadata"

	// maxContentLength is the maximum VarChar length in Milvus.
	maxContentLength = int64(65535)
)

// StoreConfig contains configuration options for Milvus vector store.
type StoreConfig struct {
	// Client is the Milvus client instance.
	// Required: must be provided, otherwise initialization will fail.
	Client *milvusclient.Client

	// CollectionName is the name of the Milvus collection.
	// Required: must be a non-empty string.
	CollectionName string

	// InitializeSchema indicates whether to automatically create the collection
	// and its vector index if they do not exist.
	// Optional: defaults to false.
	InitializeSchema bool

	// EmbeddingModel is the model used to generate vector embeddings from text.
	// Required: must be provided.
	EmbeddingModel embedding.Model

	// DocumentBatcher is responsible for batching documents before insertion.
	// Required: must be provided.
	DocumentBatcher document.Batcher

	// StoreDocumentContent determines whether to store the original document
	// text in the content field. Truncated to 65535 characters if exceeded.
	// Optional: defaults to false.
	StoreDocumentContent bool

	// MetricType is the similarity metric used when creating the vector index.
	// Optional: defaults to entity.COSINE.
	MetricType entity.MetricType
}

func (c StoreConfig) Validate() error {
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
	if c.MetricType == "" {
		c.MetricType = entity.COSINE
	}
	return nil
}

var _ vectorstore.Store = (*Store)(nil)

type Store struct {
	client               *milvusclient.Client
	embeddingModel       embedding.Model
	embeddingClient      *embedding.Client
	documentBatcher      document.Batcher
	collectionName       string
	metricType           entity.MetricType
	initializeSchema     bool
	storeDocumentContent bool
}

func NewStore(cfg StoreConfig) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(cfg.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to create embedding client: %w", err)
	}

	store := &Store{
		client:               cfg.Client,
		embeddingModel:       cfg.EmbeddingModel,
		embeddingClient:      embeddingClient,
		documentBatcher:      cfg.DocumentBatcher,
		collectionName:       cfg.CollectionName,
		metricType:           cfg.MetricType,
		initializeSchema:     cfg.InitializeSchema,
		storeDocumentContent: cfg.StoreDocumentContent,
	}

	if err = store.initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("milvus: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (v *Store) createSchema(dim int64) *entity.Schema {
	return entity.NewSchema().
		WithField(entity.NewField().
			WithName(fieldID).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(36).
			WithIsPrimaryKey(true)).
		WithField(entity.NewField().
			WithName(fieldVector).
			WithDataType(entity.FieldTypeFloatVector).
			WithDim(dim)).
		WithField(entity.NewField().
			WithName(fieldContent).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(maxContentLength)).
		WithField(entity.NewField().
			WithName(fieldMeta).
			WithDataType(entity.FieldTypeJSON))
}

func (v *Store) initialize(ctx context.Context) error {
	if !v.initializeSchema {
		return nil
	}

	exists, err := v.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(v.collectionName))
	if err != nil {
		return fmt.Errorf("milvus: failed to check collection existence: %w", err)
	}

	if !exists {
		dim := v.embeddingModel.Dimensions(ctx)
		if dim <= 0 {
			return errors.New("milvus: dimensions must be greater than zero")
		}

		schema := v.createSchema(dim)
		if err = v.client.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(v.collectionName, schema)); err != nil {
			return fmt.Errorf("milvus: failed to create collection %s: %w", v.collectionName, err)
		}

		idx := index.NewAutoIndex(v.metricType)
		indexTask, err := v.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(v.collectionName, fieldVector, idx))
		if err != nil {
			return fmt.Errorf("milvus: failed to create index on collection %s: %w", v.collectionName, err)
		}
		if err = indexTask.Await(ctx); err != nil {
			return fmt.Errorf("milvus: failed to await index creation on collection %s: %w", v.collectionName, err)
		}
	}

	loadTask, err := v.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(v.collectionName))
	if err != nil {
		return fmt.Errorf("milvus: failed to load collection %s: %w", v.collectionName, err)
	}
	if err = loadTask.Await(ctx); err != nil {
		return fmt.Errorf("milvus: failed to await collection load %s: %w", v.collectionName, err)
	}

	return nil
}

func (v *Store) buildInsertColumns(docs []*document.Document, vectors [][]float64) ([]column.Column, error) {
	n := len(docs)
	ids := make([]string, n)
	vecs := make([][]float32, n)
	contents := make([]string, n)
	metaBytes := make([][]byte, n)

	for i, doc := range docs {
		ids[i] = uuid.NewString()
		vecs[i] = math.ConvertSlice[float64, float32](vectors[i])

		if v.storeDocumentContent {
			content := doc.Text
			if int64(len(content)) > maxContentLength {
				content = content[:maxContentLength]
			}
			contents[i] = content
		}

		meta, err := json.Marshal(doc.Metadata)
		if err != nil {
			return nil, fmt.Errorf("milvus: failed to marshal metadata for document %s: %w", doc.ID, err)
		}
		metaBytes[i] = meta
	}

	dim := len(vecs[0])

	return []column.Column{
		column.NewColumnVarChar(fieldID, ids),
		column.NewColumnFloatVector(fieldVector, dim, vecs),
		column.NewColumnVarChar(fieldContent, contents),
		column.NewColumnJSONBytes(fieldMeta, metaBytes),
	}, nil
}

func (v *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("milvus: invalid create request: %w", err)
	}

	ctx, span := tracing.StartCreate(ctx, "milvus", len(req.Documents))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = v.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("milvus: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := v.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("milvus: failed to generate vectors: %w", err)
		}

		cols, err := v.buildInsertColumns(docs, vectors)
		if err != nil {
			return err
		}

		_, err = v.client.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(v.collectionName, cols...))
		if err != nil {
			return fmt.Errorf("milvus: failed to upsert %d documents to collection %s: %w",
				len(docs), v.collectionName, err)
		}
	}

	return nil
}

func (v *Store) buildDocumentsFromResults(rs milvusclient.ResultSet, minScore float64) ([]*document.Document, error) {
	docs := make([]*document.Document, 0, rs.Len())

	idCol := rs.GetColumn(fieldID)
	contentCol := rs.GetColumn(fieldContent)
	metaCol := rs.GetColumn(fieldMeta)

	for i := range rs.Len() {
		score := float64(rs.Scores[i])
		if score < minScore {
			continue
		}

		doc := &document.Document{Score: score}

		if idCol != nil {
			if id, err := idCol.GetAsString(i); err == nil {
				doc.ID = id
			}
		}

		if v.storeDocumentContent && contentCol != nil {
			if text, err := contentCol.GetAsString(i); err == nil {
				doc.Text = text
			}
		}

		if metaCol != nil {
			if raw, err := metaCol.Get(i); err == nil {
				if metaBytes, ok := raw.([]byte); ok {
					var metadata map[string]any
					if err = json.Unmarshal(metaBytes, &metadata); err == nil {
						doc.Metadata = metadata
					}
				}
			}
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (v *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("milvus: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "milvus", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = v.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to embed query text: %w", err)
	}

	queryVec := entity.FloatVector(math.ConvertSlice[float64, float32](vector))

	searchOpt := milvusclient.NewSearchOption(v.collectionName, int(req.TopK), []entity.Vector{queryVec}).
		WithANNSField(fieldVector).
		WithOutputFields(fieldID, fieldContent, fieldMeta)

	if req.Filter != nil {
		filterExpr, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("milvus: failed to convert filter: %w", err)
		}
		searchOpt = searchOpt.WithFilter(filterExpr)
	}

	results, err := v.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to search collection %s: %w", v.collectionName, err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	docs, err = v.buildDocumentsFromResults(results[0], float64(req.MinScore))
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to build documents from results: %w", err)
	}

	return docs, nil
}

func (v *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) (err error) {
	if err = req.Validate(); err != nil {
		return fmt.Errorf("milvus: invalid delete request: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "milvus")
	defer func() { tracing.Finish(span, err) }()

	var filterExpr string
	filterExpr, err = ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("milvus: failed to convert filter: %w", err)
	}

	_, err = v.client.Delete(ctx, milvusclient.NewDeleteOption(v.collectionName).WithExpr(filterExpr))
	if err != nil {
		return fmt.Errorf("milvus: failed to delete from collection %s: %w", v.collectionName, err)
	}

	return nil
}

func (v *Store) Metadata() vectorstore.StoreMetadata {
	return vectorstore.StoreMetadata{
		NativeClient: v.client,
		Provider:     Provider,
	}
}

func (v *Store) Close() error {
	return v.client.Close(context.Background())
}
