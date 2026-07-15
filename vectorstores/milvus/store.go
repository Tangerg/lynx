package milvus

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

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
	// Context is the bootstrap context used during NewStore (schema /
	// index creation when InitializeSchema is true). Per-call operations
	// (Add / Search / DeleteWhere / DeleteIDs) use their own caller-supplied ctx and
	// ignore this field. Optional: defaults to context.Background().
	Context context.Context

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
	DocumentBatcher vectorstores.Batcher

	// StoreDocumentContent determines whether to store the original document
	// text in the content field. Truncated to 65535 characters if exceeded.
	// Optional: defaults to false.
	StoreDocumentContent bool

	// MetricType is the similarity metric used when creating the vector index.
	// Optional: defaults to entity.COSINE.
	MetricType entity.MetricType
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

// ApplyDefaults fills zero fields. MetricType defaults to
// [entity.COSINE]; Context defaults to context.Background().
func (c *StoreConfig) ApplyDefaults() {
	if c.MetricType == "" {
		c.MetricType = entity.COSINE
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

type Store struct {
	client               *milvusclient.Client
	embeddingModel       embedding.Model
	embeddingClient      *embeddingclient.Client
	documentBatcher      vectorstores.Batcher
	collectionName       string
	metricType           entity.MetricType
	initializeSchema     bool
	storeDocumentContent bool
}

func NewStore(cfg StoreConfig) (*Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(cfg.EmbeddingModel)
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

	if err = store.initialize(cfg.Context); err != nil {
		return nil, fmt.Errorf("milvus: failed to initialize vector store: %w", err)
	}

	return store, nil
}

func (s *Store) createSchema(dim int64) *entity.Schema {
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

func (s *Store) initialize(ctx context.Context) error {
	if !s.initializeSchema {
		return nil
	}

	exists, err := s.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(s.collectionName))
	if err != nil {
		return fmt.Errorf("milvus: failed to check collection existence: %w", err)
	}

	if !exists {
		dimensions, err := embedding.ResolveDimensions(ctx, s.embeddingModel)
		if err != nil {
			return fmt.Errorf("milvus: resolve embedding dimensions: %w", err)
		}

		schema := s.createSchema(int64(dimensions))
		if err = s.client.CreateCollection(ctx, milvusclient.NewCreateCollectionOption(s.collectionName, schema)); err != nil {
			return fmt.Errorf("milvus: failed to create collection %s: %w", s.collectionName, err)
		}

		idx := index.NewAutoIndex(s.metricType)
		indexTask, createErr := s.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(s.collectionName, fieldVector, idx))
		if createErr != nil {
			return fmt.Errorf("milvus: failed to create index on collection %s: %w", s.collectionName, createErr)
		}
		if err = indexTask.Await(ctx); err != nil {
			return fmt.Errorf("milvus: failed to await index creation on collection %s: %w", s.collectionName, err)
		}
	}

	loadTask, err := s.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(s.collectionName))
	if err != nil {
		return fmt.Errorf("milvus: failed to load collection %s: %w", s.collectionName, err)
	}
	if err = loadTask.Await(ctx); err != nil {
		return fmt.Errorf("milvus: failed to await collection load %s: %w", s.collectionName, err)
	}

	return nil
}

func (s *Store) buildInsertColumns(docs []*document.Document, vectors [][]float64) ([]column.Column, error) {
	n := len(docs)
	ids := make([]string, n)
	vecs := make([][]float32, n)
	contents := make([]string, n)
	metaBytes := make([][]byte, n)

	for i, doc := range docs {
		ids[i] = uuid.NewString()
		vecs[i] = math.ConvertSlice[float64, float32](vectors[i])

		if s.storeDocumentContent {
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

func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "milvus", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("milvus: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("milvus: failed to generate vectors: %w", err)
		}

		cols, err := s.buildInsertColumns(docs, vectors)
		if err != nil {
			return err
		}

		_, err = s.client.Upsert(ctx, milvusclient.NewColumnBasedInsertOption(s.collectionName, cols...))
		if err != nil {
			return fmt.Errorf("milvus: failed to upsert %d documents to collection %s: %w",
				len(docs), s.collectionName, err)
		}
	}

	return nil
}

func (s *Store) buildDocumentsFromResults(rs milvusclient.ResultSet, minScore float64) ([]vectorstore.Match, error) {
	docs := make([]vectorstore.Match, 0, rs.Len())

	idCol := rs.GetColumn(fieldID)
	contentCol := rs.GetColumn(fieldContent)
	metaCol := rs.GetColumn(fieldMeta)

	for i := range rs.Len() {
		score := float64(rs.Scores[i])
		if score < minScore {
			continue
		}

		doc := &document.Document{}

		if idCol != nil {
			if id, err := idCol.GetAsString(i); err == nil {
				doc.ID = id
			}
		}

		if s.storeDocumentContent && contentCol != nil {
			if text, err := contentCol.GetAsString(i); err == nil {
				doc.Text = text
			}
		}

		if metaCol != nil {
			if raw, err := metaCol.Get(i); err == nil {
				if metaBytes, ok := raw.([]byte); ok {
					var decodedMetadata metadata.Map
					if err = json.Unmarshal(metaBytes, &decodedMetadata); err == nil {
						doc.Metadata = decodedMetadata
					}
				}
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}

	return docs, nil
}

func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("milvus: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "milvus", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to embed query text: %w", err)
	}

	queryVec := entity.FloatVector(math.ConvertSlice[float64, float32](vector))

	searchOpt := milvusclient.NewSearchOption(s.collectionName, int(req.TopK), []entity.Vector{queryVec}).
		WithANNSField(fieldVector).
		WithOutputFields(fieldID, fieldContent, fieldMeta)

	if req.Filter != nil {
		filterExpr, filterErr := ToFilter(req.Filter)
		if filterErr != nil {
			return nil, fmt.Errorf("milvus: failed to convert filter: %w", filterErr)
		}
		searchOpt = searchOpt.WithFilter(filterExpr)
	}

	results, err := s.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to search collection %s: %w", s.collectionName, err)
	}

	if len(results) == 0 {
		return nil, nil
	}

	docs, err = s.buildDocumentsFromResults(results[0], float64(req.MinScore))
	if err != nil {
		return nil, fmt.Errorf("milvus: failed to build documents from results: %w", err)
	}

	return docs, nil
}

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "milvus")
	defer func() { tracing.Finish(span, err) }()

	var filterExpr string
	filterExpr, err = ToFilter(expr)
	if err != nil {
		return fmt.Errorf("milvus: failed to convert filter: %w", err)
	}

	_, err = s.client.Delete(ctx, milvusclient.NewDeleteOption(s.collectionName).WithExpr(filterExpr))
	if err != nil {
		return fmt.Errorf("milvus: failed to delete from collection %s: %w", s.collectionName, err)
	}

	return nil
}

// DeleteIDs removes rows by primary key. WithStringIDs compiles to the
// expr `id in ["a","b"]`, so unknown ids are silently ignored (idempotent).
// An empty slice is a no-op. Implements [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "milvus")
	defer func() { tracing.Finish(span, err) }()

	_, err = s.client.Delete(ctx, milvusclient.NewDeleteOption(s.collectionName).WithStringIDs(fieldID, ids))
	if err != nil {
		return fmt.Errorf("milvus: failed to delete by ids from collection %s: %w", s.collectionName, err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.client.Close(context.Background())
}
