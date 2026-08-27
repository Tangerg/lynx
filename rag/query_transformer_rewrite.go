package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
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

func (c RewriteTransformerConfig) normalized() (RewriteTransformerConfig, error) {
	if c.TargetSearchSystem == "" {
		c.TargetSearchSystem = defaultRewriteTarget
		return c, nil
	}
	if c.TargetSearchSystem != strings.TrimSpace(c.TargetSearchSystem) {
		return RewriteTransformerConfig{}, errors.New("rag: rewrite target must not have surrounding whitespace")
	}
	return c, nil
}

var _ Transformer = (*RewriteTransformer)(nil)

// RewriteTransformer tightens a query for a configured search target.
type RewriteTransformer struct {
	prompt             textModelPrompt
	targetSearchSystem string
}

type rewritePromptVariables struct {
	Target string
	Query  string
}

// NewRewriteTransformer returns a transformer that tightens a verbose or
// ambiguous user query for a configured search target.
func NewRewriteTransformer(cfg RewriteTransformerConfig) (*RewriteTransformer, error) {
	cfg, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	prompt, err := newTextModelPrompt(
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
func (r *RewriteTransformer) Transform(ctx context.Context, query Query) (Query, error) {
	if err := query.Validate(); err != nil {
		return Query{}, err
	}

	rewritten, err := r.prompt.call(ctx, rewritePromptVariables{
		Target: r.targetSearchSystem,
		Query:  query.Text(),
	})
	if err != nil {
		return Query{}, err
	}

	return query.WithText(rewritten)
}
