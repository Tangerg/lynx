package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/model/chat"
)

// contextualDefaultTemplate is the default RAG augmentation prompt: it
// drops the retrieved docs into a Context block, asks the LLM to
// answer using only that context, and forbids "based on the
// context..." filler so the answers read more naturally.
const contextualDefaultTemplate = `Context information is below.

---------------------
{{.Context}}
---------------------

Given the context information and no prior knowledge, answer the query.

Follow these rules:

1. If the answer is not in the context, just say that you don't know.
2. Avoid statements like "Based on the context..." or "The provided information...".

Query: {{.Query}}

Answer:`

// contextualEmptyContextTemplate is the canned response when no
// documents are retrieved and AllowEmptyContext is false.
const contextualEmptyContextTemplate = `The user query is outside your knowledge base.
Politely inform the user that you can't answer it.`

// ContextualQueryAugmenterConfig configures a
// [ContextualQueryAugmenter].
type ContextualQueryAugmenterConfig struct {
	// PromptTemplate is the augmentation template. Defaults to
	// [contextualDefaultTemplate]. Custom templates must declare
	// {{.Context}} and {{.Query}}.
	PromptTemplate *chat.PromptTemplate

	// EmptyContextPromptTemplate is the response template used when no
	// documents are retrieved AND AllowEmptyContext is false. Defaults
	// to [contextualEmptyContextTemplate].
	EmptyContextPromptTemplate *chat.PromptTemplate

	// AllowEmptyContext, when true, returns the user's query unchanged
	// if no documents were retrieved instead of synthesizing the
	// empty-context fallback. Defaults to false.
	AllowEmptyContext bool
}

// validate fills the default templates and rejects invalid configs.
func (c *ContextualQueryAugmenterConfig) validate() error {
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate().WithTemplate(contextualDefaultTemplate)
	}
	if c.EmptyContextPromptTemplate == nil {
		c.EmptyContextPromptTemplate = chat.NewPromptTemplate().WithTemplate(contextualEmptyContextTemplate)
	}
	return c.PromptTemplate.RequireVariables("Context", "Query")
}

var _ QueryAugmenter = (*ContextualQueryAugmenter)(nil)

// ContextualQueryAugmenter folds retrieved documents into the user's
// query as a "context" block, producing a grounded prompt that
// reduces hallucinations. Empty contexts are handled either by
// returning the original query (AllowEmptyContext=true) or by
// synthesizing a polite refusal (AllowEmptyContext=false, the
// default).
//
// Example:
//
//	aug, _ := rag.NewContextualQueryAugmenter(rag.ContextualQueryAugmenterConfig{})
//	finalQ, err := aug.Augment(ctx, q, retrievedDocs)
type ContextualQueryAugmenter struct {
	promptTemplate             *chat.PromptTemplate
	emptyContextPromptTemplate *chat.PromptTemplate
	allowEmptyContext          bool
}

// NewContextualQueryAugmenter builds a [ContextualQueryAugmenter] from
// cfg. Returns an error when the configuration fails validation.
func NewContextualQueryAugmenter(cfg ContextualQueryAugmenterConfig) (*ContextualQueryAugmenter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &ContextualQueryAugmenter{
		promptTemplate:             cfg.PromptTemplate,
		emptyContextPromptTemplate: cfg.EmptyContextPromptTemplate,
		allowEmptyContext:          cfg.AllowEmptyContext,
	}, nil
}

// Augment renders the prompt template with the documents joined as
// context. When documents is empty, falls back to
// [ContextualQueryAugmenter.handleEmptyContext]. Honors ctx
// cancellation.
func (c *ContextualQueryAugmenter) Augment(ctx context.Context, query *Query, documents []*document.Document) (*Query, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query == nil {
		return nil, errors.New("rag.ContextualQueryAugmenter.Augment: query must not be nil")
	}

	if len(documents) == 0 {
		return c.handleEmptyContext(query)
	}

	contextTexts := make([]string, 0, len(documents))
	for _, doc := range documents {
		contextTexts = append(contextTexts, doc.Format())
	}

	rendered, err := c.promptTemplate.Clone().
		WithVariable("Context", strings.Join(contextTexts, "\n\n---\n\n")).
		WithVariable("Query", query.Text).
		Render()
	if err != nil {
		return nil, err
	}
	return NewQuery(rendered)
}

// handleEmptyContext implements the no-docs branch: pass through the
// original query (AllowEmptyContext=true) or render the empty-context
// refusal template.
func (c *ContextualQueryAugmenter) handleEmptyContext(query *Query) (*Query, error) {
	if c.allowEmptyContext {
		return query, nil
	}

	rendered, err := c.emptyContextPromptTemplate.Render()
	if err != nil {
		return nil, err
	}
	return NewQuery(rendered)
}
