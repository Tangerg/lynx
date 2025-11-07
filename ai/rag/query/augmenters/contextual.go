package augmenters

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/rag"
)

// ContextualAugmenterConfig holds the configuration for ContextualAugmenter.
type ContextualAugmenterConfig struct {
	// PromptTemplate defines how the augmented query is structured with context.
	// Optional. If not provided, a default template will be used that combines
	// the retrieved documents as context with the original query.
	PromptTemplate *chat.PromptTemplate

	// EmptyContextPromptTemplate defines the response when no documents are found.
	// Optional. If not provided, a default template will be used that politely
	// informs the user that the query is outside the knowledge base.
	EmptyContextPromptTemplate *chat.PromptTemplate

	// AllowEmptyContext determines whether to allow queries without any context.
	// Optional. Defaults to false.
	// If true, returns the original query when no documents are found.
	// If false, uses EmptyContextPromptTemplate to generate a response.
	AllowEmptyContext bool
}

func (c *ContextualAugmenterConfig) validate() error {
	if c == nil {
		return errors.New("contextual augmenter config cannot be nil")
	}

	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`Context information is below.

---------------------
{{.Context}}
---------------------

Given the context information and no prior knowledge, answer the query.

Follow these rules:

1. If the answer is not in the context, just say that you don't know.
2. Avoid statements like "Based on the context..." or "The provided information...".

Query: {{.Query}}

Answer:`,
			)
	}

	if c.EmptyContextPromptTemplate == nil {
		c.EmptyContextPromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`The user query is outside your knowledge base.
Politely inform the user that you can't answer it.`,
			)
	}

	return c.PromptTemplate.RequireVariables("Context", "Query")
}

var _ rag.QueryAugmenter = (*ContextualAugmenter)(nil)

// ContextualAugmenter augments the user query with contextual data from the content
// of the provided documents.
//
// This augmenter is useful for:
//   - Enriching queries with relevant context from retrieved documents
//   - Creating grounded prompts that prevent hallucinations
//   - Handling cases where no relevant documents are found
//   - Building context-aware question-answering systems
type ContextualAugmenter struct {
	promptTemplate             *chat.PromptTemplate
	emptyContextPromptTemplate *chat.PromptTemplate
	allowEmptyContext          bool
}

func NewContextualAugmenter(cfg *ContextualAugmenterConfig) (*ContextualAugmenter, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &ContextualAugmenter{
		promptTemplate:             cfg.PromptTemplate,
		emptyContextPromptTemplate: cfg.EmptyContextPromptTemplate,
		allowEmptyContext:          cfg.AllowEmptyContext,
	}, nil
}

func (c *ContextualAugmenter) Augment(ctx context.Context, query *rag.Query, documents []*document.Document) (*rag.Query, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	if len(documents) == 0 {
		return c.handleEmptyContext(query)
	}

	contextTexts := make([]string, 0, len(documents))
	for _, doc := range documents {
		contextTexts = append(contextTexts, doc.Format())
	}

	augmentedText, err := c.
		promptTemplate.
		Clone().
		WithVariable("Context", strings.Join(contextTexts, "\n\n---\n\n")).
		WithVariable("Query", query.Text).
		Render()
	if err != nil {
		return nil, err
	}

	return rag.NewQuery(augmentedText)
}

func (c *ContextualAugmenter) handleEmptyContext(query *rag.Query) (*rag.Query, error) {
	if c.allowEmptyContext {
		return query, nil
	}

	emptyContextText, err := c.emptyContextPromptTemplate.Render()
	if err != nil {
		return nil, err
	}

	return rag.NewQuery(emptyContextText)
}
