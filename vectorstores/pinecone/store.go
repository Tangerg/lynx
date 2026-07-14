package pinecone

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pinecone-io/go-pinecone/v4/pinecone"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/core/vectorstore/filter/ast"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const (
	Provider = "Pinecone"
)

const (
	// payloadDocumentContentKey is the metadata key for saving document content.
	payloadDocumentContentKey = "lynx:ai:vectorstore:pinecone:payload_document_content"
)

// StoreConfig contains configuration options for Pinecone vector store.
type StoreConfig struct {
	// Client is the Pinecone client instance.
	// Required: must be provided, otherwise initialization will fail.
	Client *pinecone.Client

	// IndexHost is the host URL of the Pinecone index.
	// Required: must be a non-empty string.
	// Obtain it from DescribeIndex or the Pinecone web console.
	IndexHost string

	// Namespace is the index namespace to use for all operations.
	// Optional: defaults to the default namespace if empty.
	Namespace string

	// EmbeddingModel is the model used to generate vector embeddings from text.
	// Required: must be provided.
	EmbeddingModel embedding.Model

	// DocumentBatcher is responsible for batching documents before insertion.
	// Required: must be provided.
	DocumentBatcher vectorstores.Batcher

	// StoreDocumentContent determines whether to store the original document
	// text in the metadata. When true, the content is saved under a special key.
	// Optional: defaults to false.
	StoreDocumentContent bool
}

func (c StoreConfig) Validate() error {
	if c.Client == nil {
		return ErrMissingClient
	}
	if c.IndexHost == "" {
		return ErrMissingIndexHost
	}
	if c.EmbeddingModel == nil {
		return ErrMissingEmbeddingModel
	}
	if c.DocumentBatcher == nil {
		return ErrMissingDocumentBatcher
	}
	return nil
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
	_ vectorstore.IDDeleter     = (*Store)(nil)
)

type Store struct {
	index                *pinecone.IndexConnection
	embeddingClient      *embedding.Client
	documentBatcher      vectorstores.Batcher
	storeDocumentContent bool
}

func NewStore(cfg StoreConfig) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(cfg.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to create embedding client: %w", err)
	}

	idx, err := cfg.Client.Index(pinecone.NewIndexConnParams{
		Host:      cfg.IndexHost,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to connect to index at %s: %w", cfg.IndexHost, err)
	}

	return &Store{
		index:                idx,
		embeddingClient:      embeddingClient,
		documentBatcher:      cfg.DocumentBatcher,
		storeDocumentContent: cfg.StoreDocumentContent,
	}, nil
}

func (s *Store) buildVectors(docs []*document.Document, vectors [][]float64) ([]*pinecone.Vector, error) {
	result := make([]*pinecone.Vector, len(docs))

	for i, doc := range docs {
		values := math.ConvertSlice[float64, float32](vectors[i])

		point := &pinecone.Vector{
			Id:     uuid.NewString(),
			Values: &values,
		}

		metadataValues, err := doc.Metadata.Values()
		if err != nil {
			return nil, fmt.Errorf("pinecone: decode metadata for document %s: %w", doc.ID, err)
		}
		metaMap := make(map[string]any, len(metadataValues)+1)
		for k, val := range metadataValues {
			metaMap[k] = val
		}
		if s.storeDocumentContent {
			metaMap[payloadDocumentContentKey] = doc.Text
		}

		meta, err := structpb.NewStruct(metaMap)
		if err != nil {
			return nil, fmt.Errorf("pinecone: failed to convert metadata for document %s: %w", doc.ID, err)
		}
		point.Metadata = meta

		result[i] = point
	}

	return result, nil
}

func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "pinecone", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("pinecone: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("pinecone: failed to generate vectors: %w", err)
		}

		points, err := s.buildVectors(docs, vectors)
		if err != nil {
			return err
		}

		_, err = s.index.UpsertVectors(ctx, points)
		if err != nil {
			return fmt.Errorf("pinecone: failed to upsert %d vectors: %w", len(points), err)
		}
	}

	return nil
}

func (s *Store) buildDocumentsFromScoredVectors(svs []*pinecone.ScoredVector, minScore float64) ([]vectorstore.Match, error) {
	docs := make([]vectorstore.Match, 0, len(svs))

	for _, sv := range svs {
		score := float64(sv.Score)
		if score < minScore {
			continue
		}

		doc := &document.Document{}

		if sv.Vector != nil {
			doc.ID = sv.Vector.Id

			if sv.Vector.Metadata != nil {
				metadataValues := sv.Vector.Metadata.AsMap()

				if s.storeDocumentContent {
					if text, ok := metadataValues[payloadDocumentContentKey].(string); ok {
						doc.Text = text
					}
					delete(metadataValues, payloadDocumentContentKey)
				}

				var err error
				doc.Metadata, err = metadata.FromValues(metadataValues)
				if err != nil {
					return nil, fmt.Errorf("pinecone: encode metadata: %w", err)
				}
			}
		}

		docs = append(docs, vectorstore.Match{Document: doc, Score: score})
	}

	return docs, nil
}

func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("pinecone: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "pinecone", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to embed query text: %w", err)
	}

	queryReq := &pinecone.QueryByVectorValuesRequest{
		Vector:          math.ConvertSlice[float64, float32](vector),
		TopK:            uint32(req.TopK),
		IncludeMetadata: true,
	}

	if req.Filter != nil {
		filter, filterErr := ToFilter(req.Filter)
		if filterErr != nil {
			return nil, fmt.Errorf("pinecone: failed to convert filter: %w", filterErr)
		}
		queryReq.MetadataFilter = filter
	}

	resp, err := s.index.QueryByVectorValues(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to query index: %w", err)
	}

	if resp == nil || len(resp.Matches) == 0 {
		return nil, nil
	}

	docs, err = s.buildDocumentsFromScoredVectors(resp.Matches, float64(req.MinScore))
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to build documents from results: %w", err)
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

	ctx, span := tracing.StartDelete(ctx, "pinecone")
	defer func() { tracing.Finish(span, err) }()

	var filter *structpb.Struct
	filter, err = ToFilter(expr)
	if err != nil {
		return fmt.Errorf("pinecone: failed to convert filter: %w", err)
	}

	if err = s.index.DeleteVectorsByFilter(ctx, filter); err != nil {
		return fmt.Errorf("pinecone: failed to delete vectors: %w", err)
	}

	return nil
}

// DeleteIDs removes vectors by their string ids. An empty slice is a
// no-op; unknown ids are silently ignored (idempotent). Implements
// [vectorstore.IDDeleter].
func (s *Store) DeleteIDs(ctx context.Context, ids []string) (err error) {
	if len(ids) == 0 {
		return nil
	}

	ctx, span := tracing.StartDelete(ctx, "pinecone")
	defer func() { tracing.Finish(span, err) }()

	if err = s.index.DeleteVectorsById(ctx, ids); err != nil {
		return fmt.Errorf("pinecone: failed to delete vectors by ids: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.index.Close()
}
