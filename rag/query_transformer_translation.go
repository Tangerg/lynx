package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/model/chat"
)

// translationDefaultTemplate asks the LLM to translate the query into
// the target language, returning the original unchanged when it's
// already in that language or when language detection is uncertain.
// {{.Target}} and {{.Query}} are filled at transform time.
const translationDefaultTemplate = `Given a user query, translate it to {{.Target}}.
If the query is already in {{.Target}}, return it unchanged.
If you don't know the language of the query, return it unchanged.
Do not add explanations nor any other text.

Original query: {{.Query}}

Translated query:`

// TranslationTransformerConfig configures a
// [TranslationTransformer].
type TranslationTransformerConfig struct {
	// ChatModel performs the translation. Required.
	ChatModel chat.Model

	// TargetLanguage is the language the embedding model expects —
	// "English", "Chinese", "Spanish", etc. Required.
	TargetLanguage string

	// PromptTemplate is the LLM prompt. Defaults to
	// [translationDefaultTemplate]. Custom templates must declare
	// {{.Target}} and {{.Query}}.
	PromptTemplate *chat.PromptTemplate
}

// Validate rejects invalid configs.
func (c TranslationTransformerConfig) Validate() error {
	if c.ChatModel == nil {
		return errors.New("rag.TranslationTransformerConfig: ChatModel is required")
	}
	if c.TargetLanguage == "" {
		return errors.New("rag.TranslationTransformerConfig: TargetLanguage is required")
	}
	if c.PromptTemplate != nil {
		return c.PromptTemplate.RequireVariables("Target", "Query")
	}
	return nil
}

// ApplyDefaults fills zero fields. PromptTemplate defaults to
// [translationDefaultTemplate].
func (c *TranslationTransformerConfig) ApplyDefaults() {
	if c.PromptTemplate == nil {
		c.PromptTemplate = chat.NewPromptTemplate(translationDefaultTemplate)
	}
}

var _ QueryTransformer = (*TranslationTransformer)(nil)

// TranslationTransformer asks an LLM to translate the query into
// the language the downstream embedding model is tuned for. Queries
// already in the target language pass through unchanged. Useful for
// multilingual front-ends backed by an embedding model trained on a
// single language.
type TranslationTransformer struct {
	chatClient     *chat.Client
	targetLanguage string
	promptTemplate *chat.PromptTemplate
}

// NewTranslationTransformer builds a
// [TranslationTransformer]. Returns an error when the
// configuration fails validation or the chat client cannot be
// constructed.
func NewTranslationTransformer(cfg TranslationTransformerConfig) (*TranslationTransformer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	client, err := chat.NewClient(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &TranslationTransformer{
		chatClient:     client,
		targetLanguage: cfg.TargetLanguage,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

// Transform asks the LLM to translate the query. Returns a clone with
// Text replaced by the LLM output; an empty LLM response leaves Text
// unchanged.
func (t *TranslationTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	translated, _, err := t.chatClient.
		ChatWithPromptTemplate(
			t.promptTemplate.Clone().
				WithVariable("Target", t.targetLanguage).
				WithVariable("Query", query.Text),
		).
		Call().
		Text(ctx)
	if err != nil {
		return nil, err
	}

	clone := query.Clone()
	if translated != "" {
		clone.Text = translated
	}
	return clone, nil
}
