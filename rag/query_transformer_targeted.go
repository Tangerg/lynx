package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

type targetedTextTransformer struct {
	prompt textModelPrompt
	target string
}

type targetedPromptVariables struct {
	Target string
	Query  string
}

func newTargetedTextTransformer(
	model chat.Model,
	template *chatclient.Template,
	fallback string,
	target string,
	targetLabel string,
) (targetedTextTransformer, error) {
	if strings.TrimSpace(target) == "" {
		return targetedTextTransformer{}, fmt.Errorf("rag: %s is required", targetLabel)
	}
	if target != strings.TrimSpace(target) {
		return targetedTextTransformer{}, fmt.Errorf("rag: %s must not have surrounding whitespace", targetLabel)
	}
	prompt, err := newTextModelPrompt(
		model,
		template,
		fallback,
		promptVariableTarget,
		promptVariableQuery,
	)
	if err != nil {
		return targetedTextTransformer{}, err
	}
	return targetedTextTransformer{prompt: prompt, target: target}, nil
}

func (transformer targetedTextTransformer) transform(ctx context.Context, query Query) (Query, error) {
	if err := query.Validate(); err != nil {
		return Query{}, err
	}
	text, err := transformer.prompt.call(ctx, targetedPromptVariables{
		Target: transformer.target,
		Query:  query.Text(),
	})
	if err != nil {
		return Query{}, err
	}
	return query.WithText(text)
}
