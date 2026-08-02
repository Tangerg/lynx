package bedrockkb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime/types"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
	"github.com/Tangerg/lynx/core/vectorstore"
	"github.com/Tangerg/lynx/internal/vectorstorekit/scores"
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

	// OverrideSearchType optionally selects semantic or hybrid retrieval. Amazon
	// Bedrock chooses automatically when this is empty.
	OverrideSearchType types.SearchType

	// RerankingConfiguration and ImplicitFilterConfiguration expose Bedrock's
	// provider-specific retrieval features without allowing them to override
	// SearchRequest.TopK or SearchRequest.Filter.
	RerankingConfiguration      *types.VectorSearchRerankingConfiguration
	ImplicitFilterConfiguration *types.ImplicitFilterConfiguration
}

func (c StoreConfig) Validate() error {
	if c.Client == nil {
		return errors.New("bedrockkb: Client is required")
	}
	if c.KnowledgeBaseID == "" {
		return errors.New("bedrockkb: KnowledgeBaseID is required")
	}
	switch c.OverrideSearchType {
	case "", types.SearchTypeHybrid, types.SearchTypeSemantic:
	default:
		return fmt.Errorf("bedrockkb: unsupported OverrideSearchType %q", c.OverrideSearchType)
	}
	return nil
}

var _ vectorstore.Searcher = (*Store)(nil)

// Store is a searchable Bedrock Knowledge Base. Ingestion and deletion are
// intentionally absent because the runtime API cannot perform them.
type Store struct {
	client                      *bedrockagentruntime.Client
	knowledgeBaseID             string
	overrideSearchType          types.SearchType
	rerankingConfiguration      *types.VectorSearchRerankingConfiguration
	implicitFilterConfiguration *types.ImplicitFilterConfiguration
}

func NewStore(config StoreConfig) (*Store, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Store{
		client:                      config.Client,
		knowledgeBaseID:             config.KnowledgeBaseID,
		overrideSearchType:          config.OverrideSearchType,
		rerankingConfiguration:      config.RerankingConfiguration,
		implicitFilterConfiguration: config.ImplicitFilterConfiguration,
	}, nil
}

// Search runs the Bedrock Knowledge Base Retrieve API.
func (s *Store) Search(ctx context.Context, req vectorstore.SearchRequest) (docs []vectorstore.Match, err error) {
	if err = req.Validate(); err != nil {
		return nil, fmt.Errorf("bedrockkb.Store.Search: %w", err)
	}

	defer func() {
		if err == nil {
			err = req.ValidateMatches(docs)
		}
	}()

	vectorCfg, err := s.vectorSearchConfig(req)
	if err != nil {
		return nil, err
	}
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
func (s *Store) vectorSearchConfig(req vectorstore.SearchRequest) (*types.KnowledgeBaseVectorSearchConfiguration, error) {
	topK := int32(req.TopK)
	cfg := &types.KnowledgeBaseVectorSearchConfiguration{
		NumberOfResults:             &topK,
		OverrideSearchType:          s.overrideSearchType,
		RerankingConfiguration:      s.rerankingConfiguration,
		ImplicitFilterConfiguration: s.implicitFilterConfiguration,
	}

	if req.Filter != nil {
		retrievalFilter, err := BuildRetrievalFilter(req.Filter)
		if err != nil {
			return nil, fmt.Errorf("bedrockkb.Store.Search: compile metadata filter: %w", err)
		}
		cfg.Filter = retrievalFilter
	}
	return cfg, nil
}

// toMatch converts a Bedrock retrieval result into a Lynx match.
func toMatch(r types.KnowledgeBaseRetrievalResult) (vectorstore.Match, error) {
	doc := &document.Document{}
	if r.Score == nil {
		return vectorstore.Match{}, errors.New("bedrockkb: retrieval result is missing score")
	}
	score := scores.Bounded(*r.Score)
	if r.Content != nil && r.Content.Text != nil {
		doc.Text = *r.Content.Text
	}
	if doc.Text == "" {
		return vectorstore.Match{}, errors.New("bedrockkb: retrieval result has no text content")
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
			return vectorstore.Match{}, fmt.Errorf("bedrockkb: convert metadata: %w", err)
		}
	}

	if r.DocumentId != nil {
		doc.ID = *r.DocumentId
	}
	// Some knowledge-base source types do not return DocumentId. Their
	// provider-native location is still stable and losslessly identifies the
	// retrieval source.
	if doc.ID == "" && r.Location != nil {
		location, err := json.Marshal(r.Location)
		if err != nil {
			return vectorstore.Match{}, fmt.Errorf("bedrockkb: encode result location: %w", err)
		}
		if string(location) != "{}" && string(location) != "null" {
			doc.ID = string(location)
		}
	}
	if doc.ID == "" {
		return vectorstore.Match{}, errors.New("bedrockkb: retrieval result has no stable document identity")
	}
	return vectorstore.Match{Document: doc, Score: score}, nil
}

func (s *Store) Close() error { return nil }
