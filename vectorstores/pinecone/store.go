package pinecone

import (
	"context"
	"fmt"

	"github.com/pinecone-io/go-pinecone/v4/pinecone"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/embeddingclient"
)

const (
	Provider = "Pinecone"
)

const (
	// payloadDocumentContentKey is the metadata key for saving document content.
	payloadDocumentContentKey = "lynx:ai:vectorstore:pinecone:payload_document_content"
)

// DistanceMetric records the similarity metric configured on the existing
// Pinecone index. The data-plane connection does not expose index metadata.
type DistanceMetric string

const (
	DistanceCosine    DistanceMetric = "cosine"
	DistanceDot       DistanceMetric = "dotproduct"
	DistanceEuclidean DistanceMetric = "euclidean"
)

func (metric DistanceMetric) score(raw float64) vectorstore.Score {
	switch metric {
	case DistanceCosine:
		return vectorstore.ScoreFromCosineSimilarity(raw)
	case DistanceDot:
		return vectorstore.ScoreFromInnerProduct(raw)
	case DistanceEuclidean:
		return vectorstore.ScoreFromDistance(raw)
	default:
		return vectorstore.ScoreFromValue(raw)
	}
}

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
	DocumentBatcher vectorstore.Batcher

	// DistanceMetric must match the metric used when the index was created.
	// Required because Pinecone returns metric-specific raw scores.
	DistanceMetric DistanceMetric
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
	if c.DistanceMetric == "" {
		return ErrMissingDistanceMetric
	}
	switch c.DistanceMetric {
	case DistanceCosine, DistanceDot, DistanceEuclidean:
	default:
		return fmt.Errorf("pinecone: unsupported DistanceMetric %q", c.DistanceMetric)
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
	index           *pinecone.IndexConnection
	embeddingClient embeddingclient.Client
	documentBatcher vectorstore.Batcher
	distanceMetric  DistanceMetric
}

func NewStore(config StoreConfig) (*Store, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("pinecone: create embedding client: %w", err)
	}

	idx, err := config.Client.Index(pinecone.NewIndexConnParams{
		Host:      config.IndexHost,
		Namespace: config.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("pinecone: connect to index at %s: %w", config.IndexHost, err)
	}

	return &Store{
		index:           idx,
		embeddingClient: embeddingClient,
		documentBatcher: config.DocumentBatcher,
		distanceMetric:  config.DistanceMetric,
	}, nil
}

func (s *Store) buildVectors(docs []*document.Document, vectors [][]float64) ([]*pinecone.Vector, error) {
	result := make([]*pinecone.Vector, len(docs))

	for i, doc := range docs {
		values := embedding.Float32Vector(vectors[i])

		point := &pinecone.Vector{
			Id:     doc.ID,
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
		metaMap[payloadDocumentContentKey] = doc.Text

		meta, err := structpb.NewStruct(metaMap)
		if err != nil {
			return nil, fmt.Errorf("pinecone: convert metadata for document %s: %w", doc.ID, err)
		}
		point.Metadata = meta

		result[i] = point
	}

	return result, nil
}

func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("pinecone.Store.Index: %w", err)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("pinecone: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("pinecone: embed documents: %w", err)
		}

		points, err := s.buildVectors(docs, vectors)
		if err != nil {
			return err
		}

		_, err = s.index.UpsertVectors(ctx, points)
		if err != nil {
			return fmt.Errorf("pinecone: upsert %d vectors: %w", len(points), err)
		}
	}

	return nil
}

func (s *Store) buildDocumentsFromScoredVectors(svs []*pinecone.ScoredVector, minScore vectorstore.Score) ([]*vectorstore.SearchResult, error) {
	docs := make([]*vectorstore.SearchResult, 0, len(svs))

	for i, sv := range svs {
		if sv == nil || sv.Vector == nil {
			return nil, fmt.Errorf("pinecone: query result %d is missing its vector record", i)
		}
		score := s.distanceMetric.score(float64(sv.Score))
		if score < minScore {
			continue
		}

		if sv.Vector.Id == "" {
			return nil, fmt.Errorf("pinecone: query result %d is missing its document ID", i)
		}
		if sv.Vector.Metadata == nil {
			return nil, fmt.Errorf("pinecone: query result %d is missing metadata and document text", i)
		}
		metadataValues := sv.Vector.Metadata.AsMap()
		text, ok := metadataValues[payloadDocumentContentKey].(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("pinecone: query result %d is missing document text", i)
		}
		delete(metadataValues, payloadDocumentContentKey)

		doc := &document.Document{ID: sv.Vector.Id, Text: text}
		var err error
		doc.Metadata, err = metadata.FromValues(metadataValues)
		if err != nil {
			return nil, fmt.Errorf("pinecone: decode metadata for query result %d: %w", i, err)
		}
		docs = append(docs, &vectorstore.SearchResult{Document: doc, Score: score})
	}

	return docs, nil
}

func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("pinecone.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("pinecone: embed query: %w", err)
	}

	queryReq := &pinecone.QueryByVectorValuesRequest{
		Vector:          embedding.Float32Vector(vector),
		TopK:            uint32(req.Options.TopK),
		IncludeMetadata: true,
	}

	if req.Options.Filter != nil {
		visitor := NewVisitor()
		if err := req.Options.Filter.Accept(visitor); err != nil {
			return nil, fmt.Errorf("pinecone: convert filter: %w", err)
		}
		queryReq.MetadataFilter = visitor.Result()
	}

	resp, err := s.index.QueryByVectorValues(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("pinecone: query index: %w", err)
	}

	if resp == nil || len(resp.Matches) == 0 {
		return nil, nil
	}

	docs, err = s.buildDocumentsFromScoredVectors(resp.Matches, req.Options.MinScore)
	if err != nil {
		return nil, fmt.Errorf("pinecone: build documents from results: %w", err)
	}

	return &vectorstore.SearchResponse{Results: docs}, nil
}

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("pinecone.Store.DeleteWhere: %w", err)
	}

	visitor := NewVisitor()
	if err = expr.Accept(visitor); err != nil {
		return fmt.Errorf("pinecone: convert filter: %w", err)
	}

	if err = s.index.DeleteVectorsByFilter(ctx, visitor.Result()); err != nil {
		return fmt.Errorf("pinecone: delete vectors: %w", err)
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

	if err = s.index.DeleteVectorsById(ctx, ids); err != nil {
		return fmt.Errorf("pinecone: delete vectors by ids: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	return s.index.Close()
}
