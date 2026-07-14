package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
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

// TranslationTransformerConfig configures [NewTranslationTransformer].
type TranslationTransformerConfig struct {
	// ChatModel performs the translation. Required.
	ChatModel chat.Model

	// TargetLanguage is the language the embedding model expects —
	// "English", "Chinese", "Spanish", etc. Required.
	TargetLanguage string

	// PromptTemplate is the LLM prompt. Defaults to
	// [translationDefaultTemplate]. Custom templates must declare
	// {{.Target}} and {{.Query}}.
	PromptTemplate *chatclient.Template
}

func (c *TranslationTransformerConfig) validate() error {
	if c.ChatModel == nil {
		return errors.New("rag.TranslationTransformerConfig: ChatModel is required")
	}
	if c.TargetLanguage == "" {
		return errors.New("rag.TranslationTransformerConfig: TargetLanguage is required")
	}
	if c.PromptTemplate != nil {
		return c.PromptTemplate.Require("Target", "Query")
	}
	return nil
}

func (c *TranslationTransformerConfig) applyDefaults() error {
	if c.PromptTemplate == nil {
		var err error
		c.PromptTemplate, err = chatclient.ParseTemplate(translationDefaultTemplate)
		if err != nil {
			return err
		}
	}
	return nil
}

var _ Transformer = (*translationTransformer)(nil)

type translationTransformer struct {
	chatClient     *chatclient.Client
	targetLanguage string
	promptTemplate *chatclient.Template
}

// NewTranslationTransformer returns a [Transformer] that translates queries
// into the target language expected by downstream retrieval.
func NewTranslationTransformer(cfg TranslationTransformerConfig) (Transformer, error) {
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client, err := chatclient.New(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &translationTransformer{
		chatClient:     client,
		targetLanguage: cfg.TargetLanguage,
		promptTemplate: cfg.PromptTemplate,
	}, nil
}

// Transform asks the LLM to translate the query. Returns a clone with
// Text replaced by the LLM output; an empty LLM response leaves Text
// unchanged.
func (t *translationTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if query == nil {
		return nil, ErrNilQuery
	}

	translated, err := callTemplate(ctx, t.chatClient, t.promptTemplate, map[string]any{
		"Target": t.targetLanguage,
		"Query":  query.Text,
	})
	if err != nil {
		return nil, err
	}

	return query.withModelText(translated), nil
}
