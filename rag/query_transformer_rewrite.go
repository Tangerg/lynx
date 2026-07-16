package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
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
	PromptTemplate *chatclient.Template
}

var _ Transformer = (*rewriteTransformer)(nil)

type rewriteTransformer struct {
	chatClient         *chatclient.Client
	targetSearchSystem string
	promptTemplate     *chatclient.Template
}

// NewRewriteTransformer returns a [Transformer] that tightens a verbose or
// ambiguous user query for a configured search target.
func NewRewriteTransformer(cfg RewriteTransformerConfig) (Transformer, error) {
	if cfg.ChatModel == nil {
		return nil, errors.New("rag.RewriteTransformerConfig: ChatModel is required")
	}
	if cfg.TargetSearchSystem == "" {
		cfg.TargetSearchSystem = defaultRewriteTarget
	}
	promptTemplate, err := resolvePromptTemplate(
		cfg.PromptTemplate,
		rewriteDefaultTemplate,
		"Target",
		"Query",
	)
	if err != nil {
		return nil, err
	}

	client, err := chatclient.New(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &rewriteTransformer{
		chatClient:         client,
		targetSearchSystem: cfg.TargetSearchSystem,
		promptTemplate:     promptTemplate,
	}, nil
}

// Transform asks the LLM to rewrite the query. Returns a clone with
// Text replaced by the LLM output; an empty LLM response leaves Text
// unchanged.
func (r *rewriteTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	rewritten, err := callPrompt(ctx, r.chatClient, r.promptTemplate, map[string]any{
		"Target": r.targetSearchSystem,
		"Query":  query.Text,
	})
	if err != nil {
		return nil, err
	}

	return query.withModelText(rewritten), nil
}
