package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// rewriteDefaultTemplate asks the LLM to rewrite the query to be
// concise, specific, and tuned to a particular search target.
// {{.Target}} and {{.Query}} are filled at transform time.
const rewriteDefaultTemplate = `Given a user query, rewrite it to provide better results when querying a {{.Target}}.
Remove any irrelevant information, and ensure the query is concise and specific.

Original query:
{{.Query}}

Rewritten query:`

// defaultRewriteTarget is the assumed search target when
// [RewriteTransformerConfig.TargetSearchSystem] is unset.
const defaultRewriteTarget = "vector store"

// RewriteTransformerConfig configures [NewRewriteTransformer].
type RewriteTransformerConfig struct {
	// ChatModel performs the rewrite. Required.
	ChatModel chat.Model

	// TargetSearchSystem names the downstream search engine — "vector
	// store", "web search engine", "database", etc. Defaults to
	// [defaultRewriteTarget].
	TargetSearchSystem string

	// PromptTemplate is the LLM prompt. Defaults to
	// [rewriteDefaultTemplate]. Custom templates must declare
	// {{.Target}} and {{.Query}}.
	PromptTemplate *chat.PromptTemplate
}

func (c *RewriteTransformerConfig) validate() error {
	if c.ChatModel == nil {
		return errors.New("rag.RewriteTransformerConfig: ChatModel is required")
	}
	if c.PromptTemplate != nil {
		return c.PromptTemplate.RequireVariables("Target", "Query")
	}
	return nil
}

func (c *RewriteTransformerConfig) applyDefaults() {
	if c.TargetSearchSystem == "" {
		c.TargetSearchSystem = defaultRewriteTarget
	}
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(rewriteDefaultTemplate)
	}
}

var _ Transformer = (*rewriteTransformer)(nil)

type rewriteTransformer struct {
	chatClient         *chat.Client
	targetSearchSystem string
	promptTemplate     *chat.PromptTemplate
}

// NewRewriteTransformer returns a [Transformer] that tightens a verbose or
// ambiguous user query for a configured search target.
func NewRewriteTransformer(cfg RewriteTransformerConfig) (Transformer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClient(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &rewriteTransformer{
		chatClient:         client,
		targetSearchSystem: cfg.TargetSearchSystem,
		promptTemplate:     cfg.PromptTemplate,
	}, nil
}

// Transform asks the LLM to rewrite the query. Returns a clone with
// Text replaced by the LLM output; an empty LLM response leaves Text
// unchanged.
func (r *rewriteTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	rewritten, _, err := r.chatClient.
		ChatWithPromptTemplate(
			r.promptTemplate.Clone().
				WithVariable("Target", r.targetSearchSystem).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clone := query.Clone()
	if rewritten != "" {
		clone.Text = rewritten
	}
	return clone, nil
}
