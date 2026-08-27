package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
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

func (c TranslationTransformerConfig) validate() error {
	if strings.TrimSpace(c.TargetLanguage) == "" {
		return errors.New("rag: translation target language is required")
	}
	if c.TargetLanguage != strings.TrimSpace(c.TargetLanguage) {
		return errors.New("rag: translation target language must not have surrounding whitespace")
	}
	return nil
}

var _ Transformer = (*TranslationTransformer)(nil)

// TranslationTransformer translates queries into a configured language.
type TranslationTransformer struct {
	prompt         textModelPrompt
	targetLanguage string
}

type translationPromptVariables struct {
	Target string
	Query  string
}

func NewTranslationTransformer(config TranslationTransformerConfig) (*TranslationTransformer, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	prompt, err := newTextModelPrompt(
		config.Model,
		config.PromptTemplate,
		translationDefaultTemplate,
		promptVariableTarget,
		promptVariableQuery,
	)
	if err != nil {
		return nil, err
	}

	return &TranslationTransformer{
		prompt:         prompt,
		targetLanguage: config.TargetLanguage,
	}, nil
}

// Transform asks the LLM to translate the query and returns a clone with Text
// replaced by the model output.
func (t *TranslationTransformer) Transform(ctx context.Context, query Query) (Query, error) {
	if err := query.Validate(); err != nil {
		return Query{}, err
	}

	translated, err := t.prompt.call(ctx, translationPromptVariables{
		Target: t.targetLanguage,
		Query:  query.Text(),
	})
	if err != nil {
		return Query{}, err
	}

	return query.WithText(translated)
}
