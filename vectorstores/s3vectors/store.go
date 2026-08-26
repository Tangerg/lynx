package s3vectors

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	s3vdoc "github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/embedding"
	"github.com/Tangerg/lynx/core/embeddingclient"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

const Provider = "S3Vectors"

const (
	contentMetaKey = "lynx_content"
)

// StoreConfig contains configuration options for the AWS S3 Vectors
// vector store.
type StoreConfig struct {
	// Client is the s3vectors client. Required.
	Client *s3vectors.Client

	// VectorBucketName names the S3 Vectors bucket. Required.
	VectorBucketName string

	// IndexName names the vector index inside the bucket. Required.
	IndexName string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upload. Required.
	DocumentBatcher vectorstore.Batcher

	// DistanceMetric records the metric the index was created with —
	// the store uses this only to map the raw distance returned by
	// QueryVectors into a `higher = more similar` [0, 1] score. The
	// actual metric is set on the index out of band.
	DistanceMetric DistanceMetric
}

// DistanceMetric mirrors the metric registered with the S3 Vectors
// index. The store doesn't enforce consistency — picking the wrong
// value here just produces miscalibrated scores.
type DistanceMetric string

const (
	DistanceCosine    DistanceMetric = "cosine"
	DistanceEuclidean DistanceMetric = "euclidean"
)

func (d DistanceMetric) score(distance float64) vectorstore.Score {
	switch d {
	case DistanceEuclidean:
		return vectorstore.ScoreFromDistance(distance)
	case DistanceCosine:
		fallthrough
	default:
		return vectorstore.ScoreFromCosineDistance(distance)
	}
}

func (s StoreConfig) Validate() error {
	s.applyDefaults()
	if s.Client == nil {
		return errors.New("s3vectors: Client is required")
	}
	if s.VectorBucketName == "" {
		return errors.New("s3vectors: VectorBucketName is required")
	}
	if s.IndexName == "" {
		return errors.New("s3vectors: IndexName is required")
	}
	if s.EmbeddingModel == nil {
		return errors.New("s3vectors: EmbeddingModel is required")
	}
	if s.DocumentBatcher == nil {
		return errors.New("s3vectors: DocumentBatcher is required")
	}
	switch s.DistanceMetric {
	case DistanceCosine, DistanceEuclidean:
	default:
		return fmt.Errorf("s3vectors: unsupported DistanceMetric %q", s.DistanceMetric)
	}
	return nil
}

// applyDefaults fills zero fields with documented defaults.
func (s *StoreConfig) applyDefaults() {
	s.DistanceMetric = cmp.Or(s.DistanceMetric, DistanceCosine)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
)

// Store implements vector-store capabilities with Amazon S3 Vectors.
type Store struct {
	client           *s3vectors.Client
	vectorBucketName string
	indexName        string
	embeddingClient  embeddingclient.Client
	documentBatcher  vectorstore.Batcher
	distanceMetric   DistanceMetric
}

func NewStore(config StoreConfig) (*Store, error) {
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embeddingclient.New(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: create embedding client: %w", err)
	}

	return &Store{
		client:           config.Client,
		vectorBucketName: config.VectorBucketName,
		indexName:        config.IndexName,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		distanceMetric:   config.DistanceMetric,
	}, nil
}

// Index embeds documents and PUTs them. S3 Vectors caps each
// PutVectors batch at 500 vectors, so the document batcher should
// produce shards smaller than that.
func (s *Store) Index(ctx context.Context, request *vectorstore.IndexRequest) (err error) {
	if validateErr := request.Validate(); validateErr != nil {
		return fmt.Errorf("s3vectors.Store.Index: %w", validateErr)
	}

	var batches []*vectorstore.IndexRequest
	batches, err = request.Batch(ctx, s.documentBatcher)
	if err != nil {
		return fmt.Errorf("s3vectors: batch documents: %w", err)
	}

	for _, batch := range batches {
		docs := batch.Documents
		vectors, err := s.embeddingClient.EmbedDocuments(ctx, docs)
		if err != nil {
			return fmt.Errorf("s3vectors: embed documents: %w", err)
		}

		records := make([]types.PutInputVector, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			metadataValues, err := doc.Metadata.Values()
			if err != nil {
				return fmt.Errorf("s3vectors: decode metadata for %s: %w", id, err)
			}
			meta := make(map[string]any, len(metadataValues)+1)
			for k, v := range metadataValues {
				meta[k] = v
			}
			// Stash the document text in metadata so retrieval can
			// surface it — S3 Vectors itself only stores vector + key
			// + metadata.
			meta[contentMetaKey] = doc.Text

			records = append(records, types.PutInputVector{
				Key:      aws.String(id),
				Data:     &types.VectorDataMemberFloat32{Value: embedding.Float32Vector(vectors[i])},
				Metadata: s3vdoc.NewLazyDocument(meta),
			})
		}

		if _, err := s.client.PutVectors(ctx, &s3vectors.PutVectorsInput{
			VectorBucketName: aws.String(s.vectorBucketName),
			IndexName:        aws.String(s.indexName),
			Vectors:          records,
		}); err != nil {
			return fmt.Errorf("s3vectors: PutVectors: %w", err)
		}
	}
	return nil
}

// Search runs QueryVectors with the configured filter.
func (s *Store) Search(ctx context.Context, req *vectorstore.SearchRequest) (response *vectorstore.SearchResponse, err error) {
	var docs []*vectorstore.SearchResult
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("s3vectors.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = response.ValidateFor(req)
		}
	}()

	var vector []float64
	vector, err = s.embeddingClient.EmbedText(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: embed query: %w", err)
	}
	queryVec := embedding.Float32Vector(vector)

	input := &s3vectors.QueryVectorsInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(s.indexName),
		QueryVector:      &types.VectorDataMemberFloat32{Value: queryVec},
		TopK:             aws.Int32(int32(req.Options.TopK)),
		ReturnDistance:   true,
		ReturnMetadata:   true,
	}

	if req.Options.Filter != nil {
		filterDoc, filterErr := s.buildFilter(req.Options.Filter)
		if filterErr != nil {
			return nil, filterErr
		}
		if filterDoc != nil {
			input.Filter = s3vdoc.NewLazyDocument(filterDoc)
		}
	}

	resp, err := s.client.QueryVectors(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: QueryVectors: %w", err)
	}

	docs = make([]*vectorstore.SearchResult, 0, len(resp.Vectors))
	for _, hit := range resp.Vectors {
		match, err := s.toMatch(hit, req.Options.MinScore)
		if err != nil {
			return nil, err
		}
		if match != nil {
			docs = append(docs, match)
		}
	}
	return &vectorstore.SearchResponse{Results: docs}, nil
}

// Delete enumerates ids that match the filter via QueryVectors (S3
// Vectors has no filter-based DeleteVectors) and then issues a
// DeleteVectors call.

func (s *Store) DeleteWhere(ctx context.Context, expr filter.Predicate) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = expr.Validate(); err != nil {
		return fmt.Errorf("s3vectors.Store.DeleteWhere: %w", err)
	}

	filterDoc, err := s.buildFilter(expr)
	if err != nil {
		return err
	}
	if filterDoc == nil {
		return errors.New("s3vectors: refusing to delete on empty filter")
	}

	// Use a placeholder embedding to drive the filter scan — the
	// vector itself doesn't matter when the distance is discarded.
	dimensions, err := s.embeddingClient.Dimensions(ctx)
	if err != nil {
		return fmt.Errorf("s3vectors: resolve embedding dimensions: %w", err)
	}
	probe := make([]float32, dimensions)
	const pageSize int32 = 1000
	for {
		resp, err := s.client.QueryVectors(ctx, &s3vectors.QueryVectorsInput{
			VectorBucketName: aws.String(s.vectorBucketName),
			IndexName:        aws.String(s.indexName),
			QueryVector:      &types.VectorDataMemberFloat32{Value: probe},
			TopK:             aws.Int32(pageSize),
			Filter:           s3vdoc.NewLazyDocument(filterDoc),
		})
		if err != nil {
			return fmt.Errorf("s3vectors: enumerate ids: %w", err)
		}
		if len(resp.Vectors) == 0 {
			return nil
		}
		keys := make([]string, 0, len(resp.Vectors))
		for _, v := range resp.Vectors {
			if v.Key != nil {
				keys = append(keys, *v.Key)
			}
		}
		if _, err := s.client.DeleteVectors(ctx, &s3vectors.DeleteVectorsInput{
			VectorBucketName: aws.String(s.vectorBucketName),
			IndexName:        aws.String(s.indexName),
			Keys:             keys,
		}); err != nil {
			return fmt.Errorf("s3vectors: DeleteVectors: %w", err)
		}
		if int32(len(resp.Vectors)) < pageSize {
			return nil
		}
	}
}

func (s *Store) buildFilter(filter filter.Predicate) (map[string]any, error) {
	if filter == nil {
		return nil, nil
	}
	v := NewVisitor()
	if err := filter.Accept(v); err != nil {
		return nil, fmt.Errorf("s3vectors: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toMatch(hit types.QueryOutputVector, minScore vectorstore.Score) (*vectorstore.SearchResult, error) {
	if hit.Key == nil || *hit.Key == "" {
		return nil, errors.New("s3vectors: query result is missing key")
	}
	if hit.Distance == nil {
		return nil, errors.New("s3vectors: query result is missing distance")
	}
	doc := &document.Document{ID: *hit.Key}
	score := s.distanceMetric.score(float64(*hit.Distance))
	if score < minScore {
		return nil, nil
	}

	if hit.Metadata != nil {
		var meta map[string]any
		if err := hit.Metadata.UnmarshalSmithyDocument(&meta); err != nil {
			return nil, fmt.Errorf("s3vectors: decode metadata: %w", err)
		}
		if text, ok := meta[contentMetaKey].(string); ok {
			doc.Text = text
			delete(meta, contentMetaKey)
		}
		if len(meta) > 0 {
			var err error
			doc.Metadata, err = metadata.FromValues(meta)
			if err != nil {
				return nil, fmt.Errorf("s3vectors: convert metadata: %w", err)
			}
		}
	}
	if doc.Text == "" {
		return nil, errors.New("s3vectors: query result is missing document text metadata")
	}
	return &vectorstore.SearchResult{Document: doc, Score: score}, nil
}

func (s *Store) Close() error { return nil }
