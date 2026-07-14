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
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/vectorstores/internal/tracing"
)

const Provider = "BedrockKnowledgeBase"

// StoreConfig contains configuration options for the AWS Bedrock
// Knowledge Base vector store. Bedrock manages document ingestion
// out of band (S3 data source + StartIngestionJob), so this store exposes only
// semantic search.
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
	// [vectorstore.SearchRequest.TopK] automatically.
	VectorSearchOverrides *types.KnowledgeBaseVectorSearchConfiguration
}

func (c StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("bedrockkb: Client is required")
	}
	if c.KnowledgeBaseID == "" {
		return errors.New("bedrockkb: KnowledgeBaseID is required")
	}
	return nil
}

var _ vectorstore.Searcher = (*Store)(nil)

// Store is a searchable Bedrock Knowledge Base. Ingestion and deletion are
// intentionally absent because the runtime API cannot perform them.
type Store struct {
	client          *bedrockagentruntime.Client
	knowledgeBaseID string
	vectorOverrides *types.KnowledgeBaseVectorSearchConfiguration
}

func NewStore(config StoreConfig) (*Store, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Store{
		client:          config.Client,
		knowledgeBaseID: config.KnowledgeBaseID,
		vectorOverrides: config.VectorSearchOverrides,
	}, nil
}

// Search runs the Bedrock Knowledge Base Retrieve API.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("bedrockkb: invalid search request: %w", err)
	}

	ctx, span := tracing.StartSearch(ctx, "bedrockkb", req.TopK, req.MinScore)
	defer func() { tracing.RecordSearchResult(span, err, len(docs)) }()

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

	docs = make([]vectorstore.Match, 0, len(resp.RetrievalResults))
	for _, r := range resp.RetrievalResults {
		match, err := toMatch(r)
		if err != nil {
			return nil, err
		}
		if match.Score < req.MinScore {
			continue
		}
		docs = append(docs, match)
	}
	return docs, nil
}

// vectorSearchConfig builds the per-call vector search configuration,
// layering caller-supplied overrides on top of the request defaults.
func (s *Store) vectorSearchConfig(req vectorstore.SearchRequest) *types.KnowledgeBaseVectorSearchConfiguration {
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

// toMatch converts a Bedrock retrieval result into a Lynx match.
func toMatch(r types.KnowledgeBaseRetrievalResult) (vectorstore.Match, error) {
	doc := &document.Document{}
	var score float64
	if r.Score != nil {
		score = *r.Score
	}
	if r.Content != nil && r.Content.Text != nil {
		doc.Text = *r.Content.Text
	}

	if len(r.Metadata) > 0 {
		meta := make(map[string]any, len(r.Metadata))
		for k, v := range r.Metadata {
			var decoded any
			if err := v.UnmarshalSmithyDocument(&decoded); err != nil {
				return vectorstore.Match{}, fmt.Errorf("bedrockkb: decode metadata key %s: %w", k, err)
			}
			meta[k] = decoded
		}
		var err error
		doc.Metadata, err = metadata.FromValues(meta)
		if err != nil {
			return vectorstore.Match{}, fmt.Errorf("bedrockkb: encode metadata: %w", err)
		}
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
	return vectorstore.Match{Document: doc, Score: score}, nil
}

func (s *Store) Close() error { return nil }
