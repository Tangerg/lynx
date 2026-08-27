package rag

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
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
	// Model performs the rewrite. Required.
	Model chat.Model

	// TargetSearchSystem names the downstream search engine — "vector
	// store", "web search engine", "database", etc. Defaults to
	// [defaultRewriteTarget].
	TargetSearchSystem string

	// PromptTemplate is the LLM prompt. Defaults to
	// [rewriteDefaultTemplate]. Custom templates must declare
	// {{.Target}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

var _ Transformer = (*RewriteTransformer)(nil)

// RewriteTransformer tightens a query for a configured search target.
type RewriteTransformer struct {
	prompt             modelPrompt
	targetSearchSystem string
}

type rewritePromptVariables struct {
	Target string
	Query  string
}

// NewRewriteTransformer returns a transformer that tightens a verbose or
// ambiguous user query for a configured search target.
func NewRewriteTransformer(cfg RewriteTransformerConfig) (*RewriteTransformer, error) {
	if cfg.TargetSearchSystem == "" {
		cfg.TargetSearchSystem = defaultRewriteTarget
	}
	prompt, err := newModelPrompt(
		cfg.Model,
		cfg.PromptTemplate,
		rewriteDefaultTemplate,
		promptVariableTarget,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &RewriteTransformer{
		prompt:             prompt,
		targetSearchSystem: cfg.TargetSearchSystem,
	}, nil
}

// Transform asks the LLM to rewrite the query and returns a clone with Text
// replaced by the model output.
func (r *RewriteTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	rewritten, err := r.prompt.call(ctx, rewritePromptVariables{
		Target: r.targetSearchSystem,
		Query:  query.Text(),
	})
	if err != nil {
		return nil, err
	}

	return query.WithText(rewritten)
}
