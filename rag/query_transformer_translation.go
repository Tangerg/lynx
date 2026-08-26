package rag

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
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
	// Model performs the translation. Required.
	Model chat.Model

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
	prompt         modelPrompt
	targetLanguage string
}

type translationPromptVariables struct {
	Target string
	Query  string
}

// NewTranslationTransformer returns a [Transformer] that translates queries
// into the target language expected by downstream retrieval.
func NewTranslationTransformer(cfg TranslationTransformerConfig) (Transformer, error) {
	if cfg.TargetLanguage == "" {
		return nil, errors.New("rag: translation target language is required")
	}
	prompt, err := newModelPrompt(
		cfg.Model,
		cfg.PromptTemplate,
		translationDefaultTemplate,
		promptVariableTarget,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &translationTransformer{
		prompt:         prompt,
		targetLanguage: cfg.TargetLanguage,
	}, nil
}

// Transform asks the LLM to translate the query and returns a clone with Text
// replaced by the model output.
func (t *translationTransformer) Transform(ctx context.Context, query *Query) (*Query, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}

	translated, err := t.prompt.call(ctx, translationPromptVariables{
		Target: t.targetLanguage,
		Query:  query.Text(),
	})
	if err != nil {
		return nil, err
	}

	return query.WithText(translated)
}
