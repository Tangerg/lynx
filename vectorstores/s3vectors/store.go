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
	"github.com/google/uuid"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/pkg/math"
	"github.com/Tangerg/lynx/vectorstores"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "S3Vectors"

const (
	contentMetaKey = "lynx_content"
)

// StoreConfig contains configuration options for the AWS S3 Vectors
// vector store.
type StoreConfig struct {
	// Context is unused at construction (the SDK exposes a control
	// plane for index creation but lynx leaves that to callers /
	// IaC). Kept for forward compatibility.
	Context context.Context

	// Client is the s3vectors client. Required.
	Client *s3vectors.Client

	// VectorBucketName names the S3 Vectors bucket. Required.
	VectorBucketName string

	// IndexName names the vector index inside the bucket. Required.
	IndexName string

	// EmbeddingModel produces vectors for the documents. Required.
	EmbeddingModel embedding.Model

	// DocumentBatcher batches documents before upload. Required.
	DocumentBatcher vectorstores.Batcher

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

func (c *StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("s3vectors: Client is required")
	}
	if c.VectorBucketName == "" {
		return errors.New("s3vectors: VectorBucketName is required")
	}
	if c.IndexName == "" {
		return errors.New("s3vectors: IndexName is required")
	}
	if c.EmbeddingModel == nil {
		return errors.New("s3vectors: EmbeddingModel is required")
	}
	if c.DocumentBatcher == nil {
		return errors.New("s3vectors: DocumentBatcher is required")
	}
	return nil
}

// ApplyDefaults fills zero fields with documented defaults.
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	c.DistanceMetric = cmp.Or(c.DistanceMetric, DistanceCosine)
}

var (
	_ vectorstore.Indexer       = (*Store)(nil)
	_ vectorstore.Searcher      = (*Store)(nil)
	_ vectorstore.FilterDeleter = (*Store)(nil)
)

// Store is an AWS S3 Vectors backed the vectorstore capability interfaces
// implementation.
type Store struct {
	client           *s3vectors.Client
	vectorBucketName string
	indexName        string
	embeddingModel   embedding.Model
	embeddingClient  *embedding.Client
	documentBatcher  vectorstores.Batcher
	distanceMetric   DistanceMetric
}

func NewStore(config StoreConfig) (*Store, error) {
	config.ApplyDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}

	embeddingClient, err := embedding.NewClient(config.EmbeddingModel)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: failed to create embedding client: %w", err)
	}

	return &Store{
		client:           config.Client,
		vectorBucketName: config.VectorBucketName,
		indexName:        config.IndexName,
		embeddingModel:   config.EmbeddingModel,
		embeddingClient:  embeddingClient,
		documentBatcher:  config.DocumentBatcher,
		distanceMetric:   config.DistanceMetric,
	}, nil
}

// Create embeds documents and PUTs them. S3 Vectors caps each
// PutVectors batch at 500 vectors, so the document batcher should
// produce shards smaller than that.
func (s *Store) Add(ctx context.Context, docs []*document.Document) (err error) {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	ctx, span := tracing.StartAdd(ctx, "s3vectors", len(docs))
	defer func() { tracing.Finish(span, err) }()

	var batchedDocs [][]*document.Document
	batchedDocs, err = s.documentBatcher.Batch(ctx, docs)
	if err != nil {
		return fmt.Errorf("s3vectors: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := s.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("s3vectors: failed to generate embeddings: %w", err)
		}

		records := make([]types.PutInputVector, 0, len(docs))
		for i, doc := range docs {
			id := doc.ID
			if id == "" {
				id = uuid.NewString()
			}
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
				Data:     &types.VectorDataMemberFloat32{Value: math.ConvertSlice[float64, float32](vectors[i])},
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

// Retrieve runs QueryVectors with the configured filter.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("s3vectors: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "s3vectors", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

	var vector []float64
	vector, _, err = s.embeddingClient.
		EmbedWithText(req.Query).
		Call().
		Embedding(ctx)
	if err != nil {
		return nil, fmt.Errorf("s3vectors: failed to embed query: %w", err)
	}
	queryVec := math.ConvertSlice[float64, float32](vector)

	input := &s3vectors.QueryVectorsInput{
		VectorBucketName: aws.String(s.vectorBucketName),
		IndexName:        aws.String(s.indexName),
		QueryVector:      &types.VectorDataMemberFloat32{Value: queryVec},
		TopK:             aws.Int32(int32(req.TopK)),
		ReturnDistance:   true,
		ReturnMetadata:   true,
	}

	if req.Filter != nil {
		filterDoc, filterErr := s.buildFilter(req.Filter)
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

	docs = make([]vectorstore.Match, 0, len(resp.Vectors))
	for _, hit := range resp.Vectors {
		match, err := s.toMatch(hit, req.MinScore)
		if err != nil {
			return nil, err
		}
		if match != nil {
			docs = append(docs, *match)
		}
	}
	return docs, nil
}

// Delete enumerates ids that match the filter via QueryVectors (S3
// Vectors has no filter-based DeleteVectors) and then issues a
// DeleteVectors call.
func (s *Store) DeleteWhere(ctx context.Context, expr filter.Expr) (err error) {
	if expr == nil {
		return vectorstore.ErrMissingFilter
	}
	if err = filter.Validate(expr); err != nil {
		return fmt.Errorf("invalid delete filter: %w", err)
	}

	ctx, span := tracing.StartDelete(ctx, "s3vectors")
	defer func() { tracing.Finish(span, err) }()

	filterDoc, err := s.buildFilter(expr)
	if err != nil {
		return err
	}
	if filterDoc == nil {
		return errors.New("s3vectors: refusing to delete on empty filter")
	}

	// Use a placeholder embedding to drive the filter scan — the
	// vector itself doesn't matter when the distance is discarded.
	probe := make([]float32, 0)
	if dim := embedding.GetDimensions(ctx, s.embeddingModel); dim > 0 {
		probe = make([]float32, dim)
	}
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

func (s *Store) buildFilter(filter filter.Expr) (map[string]any, error) {
	if filter == nil {
		return nil, nil
	}
	v := NewVisitor()
	if err := v.Visit(filter); err != nil {
		return nil, fmt.Errorf("s3vectors: convert filter: %w", err)
	}
	return v.Result(), nil
}

func (s *Store) toMatch(hit types.QueryOutputVector, minScore float64) (*vectorstore.Match, error) {
	doc := &document.Document{}
	var score float64
	if hit.Key != nil {
		doc.ID = *hit.Key
	}
	if hit.Distance != nil {
		score = s.distanceToScore(float64(*hit.Distance))
		if score < minScore {
			return nil, nil
		}
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
				return nil, fmt.Errorf("s3vectors: encode metadata: %w", err)
			}
		}
	}
	return &vectorstore.Match{Document: doc, Score: score}, nil
}

func (s *Store) distanceToScore(distance float64) float64 {
	switch s.distanceMetric {
	case DistanceEuclidean:
		return 1.0 / (1.0 + distance)
	case DistanceCosine:
		fallthrough
	default:
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

func (s *Store) Close() error { return nil }
