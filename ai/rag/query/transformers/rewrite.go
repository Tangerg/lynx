package transformers

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
)

// RewriteTransformerConfig holds the configuration for RewriteTransformer.
type RewriteTransformerConfig struct {
	// ChatModel is the language model used for query rewriting.
	// Required.
	ChatModel chat.Model

	// TargetSearchSystem specifies the target system for which the query is optimized.
	// Optional. Defaults to "vector store" if not provided.
	// Examples: "vector store", "web search engine", "database", etc.
	TargetSearchSystem string

	// PromptTemplate defines how the rewriting prompt is structured.
	// Optional. If not provided, a default template will be used that optimizes
	// the query for the target search system.
	PromptTemplate *chat.PromptTemplate
}

func (c *RewriteTransformerConfig) validate() error {
	if c == nil {
		return errors.New("rewrite transformer config cannot be nil")
	}

	if c.ChatModel == nil {
		return errors.New("rewrite transformer config: chat model is required")
	}

	if c.TargetSearchSystem == "" {
		c.TargetSearchSystem = "vector store"
	}

	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`Given a user query, rewrite it to provide better results when querying a {{.Target}}.
Remove any irrelevant information, and ensure the query is concise and specific.

Original query:
{{.Query}}

Rewritten query:`,
			)
	}

	return c.PromptTemplate.RequireVariables("Target", "Query")
}

var _ rag.QueryTransformer = (*RewriteTransformer)(nil)

// RewriteTransformer uses a large language model to rewrite a user query to
// provide better results when querying a target system, such as a vector store
// or a web search engine.
//
// This transformer is useful when the user query is verbose, ambiguous, or contains
// irrelevant information that may affect the quality of the search results. It helps to:
//   - Remove noise and irrelevant details from the query
//   - Make queries more concise and specific
//   - Optimize queries for the target search system
//   - Improve search accuracy and relevance
type RewriteTransformer struct {
	chatClient         *chat.Client
	targetSearchSystem string
	promptTemplate     *chat.PromptTemplate
}

func NewRewriteTransformer(cfg *RewriteTransformerConfig) (*RewriteTransformer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClientWithModel(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &RewriteTransformer{
		chatClient:         client,
		targetSearchSystem: cfg.TargetSearchSystem,
		promptTemplate:     cfg.PromptTemplate,
	}, nil
}

func (r *RewriteTransformer) Transform(ctx context.Context, query *rag.Query) (*rag.Query, error) {
	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	rewrittenText, _, err := r.
		chatClient.
		ChatPromptTemplate(
			r.promptTemplate.
				Clone().
				WithVariable("Target", r.targetSearchSystem).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clonedQuery := query.Clone()
	if rewrittenText != "" {
		clonedQuery.Text = rewrittenText
	}

	return clonedQuery, nil
}
