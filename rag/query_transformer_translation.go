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

var _ Transformer = (*translationTransformer)(nil)

type translationTransformer struct {
	chatClient     *chatclient.Client
	targetLanguage string
	promptTemplate *chatclient.Template
}

type translationPromptVariables struct {
	Target string
	Query  string
}

// NewTranslationTransformer returns a [Transformer] that translates queries
// into the target language expected by downstream retrieval.
func NewTranslationTransformer(cfg TranslationTransformerConfig) (Transformer, error) {
	if cfg.ChatModel == nil {
		return nil, errors.New("rag: translation transformer requires a chat model")
	}
	if cfg.TargetLanguage == "" {
		return nil, errors.New("rag: translation target language is required")
	}
	promptTemplate, err := resolvePromptTemplate(
		cfg.PromptTemplate,
		translationDefaultTemplate,
		promptVariableTarget,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	client, err := chatclient.New(cfg.ChatModel)
	if err != nil {
		return nil, err
	}

	return &translationTransformer{
		chatClient:     client,
		targetLanguage: cfg.TargetLanguage,
		promptTemplate: promptTemplate,
	}, nil
}

// Transform asks the LLM to translate the query and returns a clone with Text
// replaced by the model output.
func (t *translationTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	translated, err := callPrompt(ctx, t.chatClient, t.promptTemplate, translationPromptVariables{
		Target: t.targetLanguage,
		Query:  query.Text(),
	})
	if err != nil {
		return nil, err
	}

	return query.WithText(translated)
}
