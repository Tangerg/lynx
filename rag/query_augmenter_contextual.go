package rag

import (
	"context"
	"strings"

	"github.com/Tangerg/lynx/core/chatclient"
	"github.com/Tangerg/lynx/core/document"
	"github.com/samber/lo"
)

// DocumentFormatter renders one retrieved document for contextual RAG.
type DocumentFormatter interface {
	Format(*document.Document) (string, error)
}

// DocumentFormatterFunc adapts a function to [DocumentFormatter].
type DocumentFormatterFunc func(*document.Document) (string, error)

func (d DocumentFormatterFunc) Format(doc *document.Document) (string, error) { return d(doc) }

type textDocumentFormatter struct{}

func (textDocumentFormatter) Format(doc *document.Document) (string, error) { return doc.Text, nil }

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
	Formatter DocumentFormatter
}

var _ Augmenter = (*ContextualAugmenter)(nil)

// ContextualAugmenter folds retrieved documents into a contextual query.
type ContextualAugmenter struct {
	promptTemplate             *chatclient.Template
	emptyContextPromptTemplate *chatclient.Template
	allowEmptyContext          bool
	formatter                  DocumentFormatter
}

type contextualPromptVariables struct {
	Context string
	Query   string
}

// NewContextualAugmenter returns an augmenter that folds retrieved
// documents into the query text as a context block.
func NewContextualAugmenter(cfg ContextualAugmenterConfig) (*ContextualAugmenter, error) {
	promptTemplate, err := resolvePromptTemplate(
		cfg.PromptTemplate,
		contextualDefaultTemplate,
		promptVariableContext,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}
	emptyContextPromptTemplate, err := resolvePromptTemplate(
		cfg.EmptyContextPromptTemplate,
		contextualEmptyContextTemplate,
	)
	if err != nil {
		return nil, err
	}
	formatter := cfg.Formatter
	if lo.IsNil(formatter) {
		formatter = textDocumentFormatter{}
	}

	return &ContextualAugmenter{
		promptTemplate:             promptTemplate,
		emptyContextPromptTemplate: emptyContextPromptTemplate,
		allowEmptyContext:          cfg.AllowEmptyContext,
		formatter:                  formatter,
	}, nil
}

// Augment renders the prompt template with the documents joined as
// context. When documents is empty, falls back to
// [ContextualAugmenter.handleEmptyContext]. Honors ctx
// cancellation.
func (c *ContextualAugmenter) Augment(ctx context.Context, query *Query, documents []Candidate) (Augmentation, error) {
	if err := ctx.Err(); err != nil {
		return Augmentation{}, err
	}
	if err := query.Validate(); err != nil {
		return Augmentation{}, err
	}

	if len(documents) == 0 {
		return c.handleEmptyContext(query)
	}

	contextTexts := make([]string, 0, len(documents))
	for _, candidate := range documents {
		if err := candidate.Validate(); err != nil {
			return Augmentation{}, err
		}
		formatted, err := c.formatter.Format(candidate.Document)
		if err != nil {
			return Augmentation{}, err
		}
		contextTexts = append(contextTexts, formatted)
	}

	rendered, err := c.promptTemplate.Render(contextualPromptVariables{
		Context: strings.Join(contextTexts, "\n\n---\n\n"),
		Query:   query.Text(),
	})
	if err != nil {
		return Augmentation{}, err
	}
	return NewAugmentation(rendered)
}

// handleEmptyContext implements the no-docs branch: pass through the
// original query (AllowEmptyContext=true) or render the empty-context
// refusal template.
func (c *ContextualAugmenter) handleEmptyContext(query *Query) (Augmentation, error) {
	if c.allowEmptyContext {
		return NewAugmentation(query.Text())
	}

	rendered, err := c.emptyContextPromptTemplate.Render(nil)
	if err != nil {
		return Augmentation{}, err
	}
	return NewAugmentation(rendered)
}
