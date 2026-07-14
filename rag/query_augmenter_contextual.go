package rag

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/document"
)

// Formatter is the narrow rendering policy required by contextual RAG.
type Formatter interface {
	Format(*document.Document) (string, error)
}

// FormatterFunc adapts a function to Formatter.
type FormatterFunc func(*document.Document) (string, error)

func (f FormatterFunc) Format(doc *document.Document) (string, error) { return f(doc) }

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

// ContextualAugmenterConfig configures [NewContextualAugmenter].
type ContextualAugmenterConfig struct {
	// PromptTemplate is the augmentation template. Defaults to
	// [contextualDefaultTemplate]. Custom templates must declare
	// {{.Context}} and {{.Query}}.
	PromptTemplate *chatclient.Template

	// EmptyContextPromptTemplate is the response template used when no
	// documents are retrieved AND AllowEmptyContext is false. Defaults
	// to [contextualEmptyContextTemplate].
	EmptyContextPromptTemplate *chatclient.Template

	// AllowEmptyContext, when true, returns the user's query unchanged
	// if no documents were retrieved instead of synthesizing the
	// empty-context fallback. Defaults to false.
	AllowEmptyContext bool

	// Formatter renders each retrieved document. It defaults to Text only.
	Formatter Formatter
}

func (c *ContextualAugmenterConfig) applyDefaults() error {
	var err error
	if c.PromptTemplate == nil {
		c.PromptTemplate, err = chatclient.ParseTemplate(contextualDefaultTemplate)
		if err != nil {
			return err
		}
	}
	if c.EmptyContextPromptTemplate == nil {
		c.EmptyContextPromptTemplate, err = chatclient.ParseTemplate(contextualEmptyContextTemplate)
		if err != nil {
			return err
		}
	}
	if c.Formatter == nil {
		c.Formatter = FormatterFunc(func(doc *document.Document) (string, error) {
			if doc == nil {
				return "", nil
			}
			return doc.Text, nil
		})
	}
	return nil
}

func (c *ContextualAugmenterConfig) validate() error {
	if c.PromptTemplate == nil {
		return nil
	}
	return c.PromptTemplate.Require("Context", "Query")
}

var _ Augmenter = (*contextualAugmenter)(nil)

type contextualAugmenter struct {
	promptTemplate             *chatclient.Template
	emptyContextPromptTemplate *chatclient.Template
	allowEmptyContext          bool
	formatter                  Formatter
}

// NewContextualAugmenter returns an [Augmenter] that folds retrieved
// documents into the query text as a context block.
func NewContextualAugmenter(cfg ContextualAugmenterConfig) (Augmenter, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &contextualAugmenter{
		promptTemplate:             cfg.PromptTemplate,
		emptyContextPromptTemplate: cfg.EmptyContextPromptTemplate,
		allowEmptyContext:          cfg.AllowEmptyContext,
		formatter:                  cfg.Formatter,
	}, nil
}

// Augment renders the prompt template with the documents joined as
// context. When documents is empty, falls back to
// [contextualAugmenter.handleEmptyContext]. Honors ctx
// cancellation.
func (c *contextualAugmenter) Augment(ctx context.Context, query *Query, documents []Candidate) (*Query, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query == nil {
		return nil, ErrNilQuery
	}

	if len(documents) == 0 {
		return c.handleEmptyContext(query)
	}

	contextTexts := make([]string, 0, len(documents))
	for _, candidate := range documents {
		formatted, err := c.formatter.Format(candidate.Document)
		if err != nil {
			return nil, err
		}
		contextTexts = append(contextTexts, formatted)
	}

	rendered, err := c.promptTemplate.Render(map[string]any{
		"Context": strings.Join(contextTexts, "\n\n---\n\n"),
		"Query":   query.Text,
	})
	if err != nil {
		return nil, err
	}
	return query.withText(rendered)
}

// handleEmptyContext implements the no-docs branch: pass through the
// original query (AllowEmptyContext=true) or render the empty-context
// refusal template.
func (c *contextualAugmenter) handleEmptyContext(query *Query) (*Query, error) {
	if c.allowEmptyContext {
		return query, nil
	}

	rendered, err := c.emptyContextPromptTemplate.Render(nil)
	if err != nil {
		return nil, err
	}
	return query.withText(rendered)
}
