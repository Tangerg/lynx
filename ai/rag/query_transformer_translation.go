package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
)

// TranslationQueryTransformerConfig holds the configuration for TranslationQueryTransformer.
type TranslationQueryTransformerConfig struct {
	// ChatModel is the language model used for query translation.
	// Required.
	ChatModel chat.Model

	// TargetLanguage specifies the language to which the query should be translated.
	// Required. Should match the language supported by the embedding model.
	// Examples: "English", "Chinese", "Spanish", etc.
	TargetLanguage string

	// PromptTemplate defines how the translation prompt is structured.
	// Optional. If not provided, a default template will be used that translates
	// the query to the target language while preserving queries already in the target language.
	PromptTemplate *chat.PromptTemplate
}

func (c *TranslationQueryTransformerConfig) validate() error {
	if c == nil {
		return errors.New("translation transformer config cannot be nil")
	}

	if c.ChatModel == nil {
		return errors.New("translation transformer config: chat model is required")
	}

	if c.TargetLanguage == "" {
		return errors.New("translation transformer config: target language is required")
	}

	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.
			NewPromptTemplate().
			WithTemplate(
				`Given a user query, translate it to {{.Target}}.
If the query is already in {{.Target}}, return it unchanged.
If you don't know the language of the query, return it unchanged.
Do not add explanations nor any other text.

Original query: {{.Query}}

Translated query:`,
			)
	}

	return c.PromptTemplate.RequireVariables("Target", "Query")
}

var _ QueryTransformer = (*TranslationQueryTransformer)(nil)

// TranslationQueryTransformer uses a large language model to translate a query to a target
// language that is supported by the embedding model used to generate the document embeddings.
// If the query is already in the target language, it is returned unchanged. If the language
// of the query is unknown, it is also returned unchanged.
//
// This transformer is useful when the embedding model is trained on a specific language
// and the user query is in a different language. It helps to:
//   - Bridge language gaps between user queries and document embeddings
//   - Ensure queries are in the same language as the indexed documents
//   - Improve retrieval accuracy for multilingual systems
//   - Preserve queries that are already in the correct language
type TranslationQueryTransformer struct {
	chatClient     *chat.Client
	targetLanguage string
	promptTemplate *chat.PromptTemplate
}

func NewTranslationQueryTransformer(cfg *TranslationQueryTransformerConfig) (*TranslationQueryTransformer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClientWithModel(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &TranslationQueryTransformer{
		chatClient:     client,
		targetLanguage: cfg.TargetLanguage,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

func (t *TranslationQueryTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, errors.New("query cannot be nil")
	}

	translatedText, _, err := t.
		chatClient.
		ChatPromptTemplate(
			t.promptTemplate.
				Clone().
				WithVariable("Target", t.targetLanguage).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clonedQuery := query.Clone()
	if translatedText != "" {
		clonedQuery.Text = translatedText
	}

	return clonedQuery, nil
}
