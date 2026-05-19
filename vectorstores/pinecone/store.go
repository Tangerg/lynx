package pinecone

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pinecone-io/go-pinecone/v4/pinecone"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/embedding"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/pkg/math"
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
	DocumentBatcher document.Batcher

	// StoreDocumentContent determines whether to store the original document
	// text in the metadata. When true, the content is saved under a special key.
	// Optional: defaults to false.
	StoreDocumentContent bool
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return ErrNilConfig
	}
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

var _ vectorstore.Store = (*Store)(nil)

type Store struct {
	index                *pinecone.IndexConnection
	embeddingClient      *embedding.Client
	documentBatcher      document.Batcher
	storeDocumentContent bool
}

func NewStore(cfg *StoreConfig) (*Store, error) {
	if err := cfg.validate(); err != nil {
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

func (v *Store) buildVectors(docs []*document.Document, vectors [][]float64) ([]*pinecone.Vector, error) {
	result := make([]*pinecone.Vector, len(docs))

	for i, doc := range docs {
		values := math.ConvertSlice[float64, float32](vectors[i])

		point := &pinecone.Vector{
			Id:     uuid.NewString(),
			Values: &values,
		}

		metaMap := make(map[string]interface{}, len(doc.Metadata)+1)
		for k, val := range doc.Metadata {
			metaMap[k] = val
		}
		if v.storeDocumentContent {
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

func (v *Store) Create(ctx context.Context, req *vectorstore.CreateRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("pinecone: invalid create request: %w", err)
	}

	batchedDocs, err := v.documentBatcher.Batch(ctx, req.Documents)
	if err != nil {
		return fmt.Errorf("pinecone: failed to batch documents: %w", err)
	}

	for _, docs := range batchedDocs {
		vectors, _, err := v.embeddingClient.
			EmbedWithDocuments(docs).
			Call().
			Embeddings(ctx)
		if err != nil {
			return fmt.Errorf("pinecone: failed to generate vectors: %w", err)
		}

		points, err := v.buildVectors(docs, vectors)
		if err != nil {
			return err
		}

		_, err = v.index.UpsertVectors(ctx, points)
		if err != nil {
			return fmt.Errorf("pinecone: failed to upsert %d vectors: %w", len(points), err)
		}
	}

	return nil
}

func (v *Store) buildDocumentsFromScoredVectors(svs []*pinecone.ScoredVector, minScore float64) ([]*document.Document, error) {
	docs := make([]*document.Document, 0, len(svs))

	for _, sv := range svs {
		score := float64(sv.Score)
		if score < minScore {
			continue
		}

		doc := &document.Document{Score: score}

		if sv.Vector != nil {
			doc.ID = sv.Vector.Id

			if sv.Vector.Metadata != nil {
				metadata := sv.Vector.Metadata.AsMap()

				if v.storeDocumentContent {
					if text, ok := metadata[payloadDocumentContentKey].(string); ok {
						doc.Text = text
					}
					delete(metadata, payloadDocumentContentKey)
				}

				doc.Metadata = metadata
			}
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (v *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) ([]*document.Document, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("pinecone: invalid retrieval request: %w", err)
	}

	vector, _, err := v.embeddingClient.
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
		filter, err := ToFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("pinecone: failed to convert filter: %w", err)
		}
		queryReq.MetadataFilter = filter
	}

	resp, err := v.index.QueryByVectorValues(ctx, queryReq)
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to query index: %w", err)
	}

	if resp == nil || len(resp.Matches) == 0 {
		return nil, nil
	}

	docs, err := v.buildDocumentsFromScoredVectors(resp.Matches, float64(req.MinScore))
	if err != nil {
		return nil, fmt.Errorf("pinecone: failed to build documents from results: %w", err)
	}

	return docs, nil
}

func (v *Store) Delete(ctx context.Context, req *vectorstore.DeleteRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("pinecone: invalid delete request: %w", err)
	}

	filter, err := ToFilter(req.Filter)
	if err != nil {
		return fmt.Errorf("pinecone: failed to convert filter: %w", err)
	}

	if err = v.index.DeleteVectorsByFilter(ctx, filter); err != nil {
		return fmt.Errorf("pinecone: failed to delete vectors: %w", err)
	}

	return nil
}

func (v *Store) Metadata() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: v.index,
		Provider:     Provider,
	}
}

func (v *Store) Close() error {
	return v.index.Close()
}
