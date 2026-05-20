package bedrockkb

import (
	"context"
	"encoding/json"
	"fmt"

	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "BedrockKnowledgeBase"

// StoreConfig contains configuration options for the AWS Bedrock
// Knowledge Base vector store. Bedrock manages document ingestion
// out of band (S3 data source + StartIngestionJob), so this store is
// effectively read-only — [Store.Create] and [Store.Delete] return
// [ErrUnsupported].
type StoreConfig struct {
	// Client is the bedrockagentruntime client. Required.
	Client *bedrockagentruntime.Client

	// KnowledgeBaseID identifies the knowledge base to query.
	// Required.
	KnowledgeBaseID string

	// VectorSearchOverrides lets callers tweak the
	// VectorSearchConfiguration sent with each Retrieve call —
	// number of results / override search type / metadata filter.
	// Optional; the store fills in NumberOfResults from
	// [vectorstore.RetrievalRequest.TopK] automatically.
	VectorSearchOverrides *types.KnowledgeBaseVectorSearchConfiguration
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("bedrockkb: config must not be nil")
	}
	if c.Client == nil {
		return errors.New("bedrockkb: Client is required")
	}
	if c.KnowledgeBaseID == "" {
		return errors.New("bedrockkb: KnowledgeBaseID is required")
	}
	return nil
}

var _ vectorstore.Store = (*Store)(nil)

// Store is a Bedrock Knowledge Base backed [vectorstore.Store]
// implementation.
type Store struct {
	client          *bedrockagentruntime.Client
	knowledgeBaseID string
	vectorOverrides *types.KnowledgeBaseVectorSearchConfiguration
}


func NewStore(config *StoreConfig) (*Store, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Store{
		client:          config.Client,
		knowledgeBaseID: config.KnowledgeBaseID,
		vectorOverrides: config.VectorSearchOverrides,
	}, nil
}

// Create returns [ErrUnsupported]. Bedrock Knowledge Bases ingest
// documents via configured data sources (S3, Confluence, etc.), not
// the runtime API.
func (s *Store) Create(_ context.Context, _ *vectorstore.CreateRequest) error {
	return ErrUnsupported
}

// Delete returns [ErrUnsupported] for the same reason as
// [Store.Create].
func (s *Store) Delete(_ context.Context, _ *vectorstore.DeleteRequest) error {
	return ErrUnsupported
}

// Retrieve runs the Bedrock Knowledge Base Retrieve API.
func (s *Store) Retrieve(ctx context.Context, req *vectorstore.RetrievalRequest) (docs []*document.Document, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("bedrockkb: invalid retrieval request: %w", err)
	}

	ctx, span := tracing.StartRetrieve(ctx, "bedrockkb", req.TopK, req.MinScore)
	defer func() { tracing.RecordRetrieveResult(span, err, len(docs)) }()

	vectorCfg := s.vectorSearchConfig(req)
	retrievalCfg := &types.KnowledgeBaseRetrievalConfiguration{
		VectorSearchConfiguration: vectorCfg,
	}

	input := &bedrockagentruntime.RetrieveInput{
		KnowledgeBaseId:        aws.String(s.knowledgeBaseID),
		RetrievalQuery:         &types.KnowledgeBaseQuery{Text: aws.String(req.Query)},
		RetrievalConfiguration: retrievalCfg,
	}

	var resp *bedrockagentruntime.RetrieveOutput
	resp, err = s.client.Retrieve(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrockkb: retrieve: %w", err)
	}

	docs = make([]*document.Document, 0, len(resp.RetrievalResults))
	for _, r := range resp.RetrievalResults {
		doc, err := toDocument(r)
		if err != nil {
			return nil, err
		}
		if doc.Score < req.MinScore {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// vectorSearchConfig builds the per-call vector search configuration,
// layering caller-supplied overrides on top of the request defaults.
func (s *Store) vectorSearchConfig(req *vectorstore.RetrievalRequest) *types.KnowledgeBaseVectorSearchConfiguration {
	topK := int32(req.TopK)
	cfg := &types.KnowledgeBaseVectorSearchConfiguration{
		NumberOfResults: &topK,
	}
	if s.vectorOverrides != nil {
		if s.vectorOverrides.NumberOfResults != nil {
			cfg.NumberOfResults = s.vectorOverrides.NumberOfResults
		}
		cfg.OverrideSearchType = s.vectorOverrides.OverrideSearchType
		cfg.RerankingConfiguration = s.vectorOverrides.RerankingConfiguration
		cfg.ImplicitFilterConfiguration = s.vectorOverrides.ImplicitFilterConfiguration
		cfg.Filter = s.vectorOverrides.Filter
	}

	if req.Filter != nil {
		filter, err := BuildRetrievalFilter(req.Filter)
		if err == nil && filter != nil {
			cfg.Filter = filter
		}
	}
	return cfg
}

// toDocument converts a Bedrock retrieval result into a Lynx
// document.
func toDocument(r types.KnowledgeBaseRetrievalResult) (*document.Document, error) {
	doc := &document.Document{}
	if r.Score != nil {
		doc.Score = *r.Score
	}
	if r.Content != nil && r.Content.Text != nil {
		doc.Text = *r.Content.Text
	}

	if len(r.Metadata) > 0 {
		meta := make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			var decoded any
			if err := v.UnmarshalSmithyDocument(&decoded); err != nil {
				return nil, fmt.Errorf("bedrockkb: decode metadata key %s: %w", k, err)
			}
			meta[k] = decoded
		}
		doc.Metadata = meta
	}

	// Bedrock doesn't expose stable per-row identifiers; use the
	// source URI / S3 path when present, otherwise mint a stable
	// fingerprint of the content.
	if r.Location != nil {
		if loc, err := json.Marshal(r.Location); err == nil && len(loc) > 0 {
			doc.ID = string(loc)
		}
	}
	if doc.ID == "" {
		doc.ID = doc.Text
	}
	return doc, nil
}

func (s *Store) Metadata() vectorstore.StoreInfo {
	return vectorstore.StoreInfo{
		NativeClient: s.client,
		Provider:     Provider,
	}
}


func (s *Store) Close() error { return nil }
