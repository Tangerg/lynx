package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

const (
	promptVariableContext    = "Context"
	promptVariableCandidates = "Candidates"
	promptVariableHistory    = "History"
	promptVariableNumber     = "Number"
	promptVariableQuery      = "Query"
	promptVariableTarget     = "Target"
)

// ErrEmptyModelOutput reports a successful text model call that produced no
// usable query text. A requested transformation must not silently become an
// identity operation.
var ErrEmptyModelOutput = errors.New("rag: model returned empty query text")

// modelPrompt owns the common template and typed generation boundary used by
// every model-backed RAG component.
type modelPrompt[T any] struct {
	generation chatclient.Generation[T]
	template   *chatclient.Template
}

type textModelPrompt struct {
	modelPrompt[string]
}

func newModelPrompt[T any](
	model chat.Model,
	format chatclient.OutputFormat[T],
	template *chatclient.Template,
	fallback string,
	required ...string,
) (modelPrompt[T], error) {
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		return modelPrompt[T]{}, err
	}
	template, err = resolvePromptTemplate(template, fallback, required...)
	if err != nil {
		return modelPrompt[T]{}, err
	}
	return modelPrompt[T]{generation: client.Output(format), template: template}, nil
}

func newTextModelPrompt(
	model chat.Model,
	template *chatclient.Template,
	fallback string,
	required ...string,
) (textModelPrompt, error) {
	prompt, err := newModelPrompt(model, chatclient.Text(), template, fallback, required...)
	if err != nil {
		return textModelPrompt{}, err
	}
	return textModelPrompt{modelPrompt: prompt}, nil
}

func resolvePromptTemplate(current *chatclient.Template, fallback string, required ...string) (*chatclient.Template, error) {
	if current == nil {
		var err error
		current, err = chatclient.ParseTemplate(fallback)
		if err != nil {
			return nil, err
		}
	}
	if err := current.Require(required...); err != nil {
		return nil, err
	}
	return current, nil
}

func (m modelPrompt[T]) call(ctx context.Context, data any) (T, error) {
	var zero T
	message, err := m.template.UserMessage(data)
	if err != nil {
		return zero, err
	}
	return m.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
}

func (m textModelPrompt) call(ctx context.Context, data any) (string, error) {
	text, err := m.modelPrompt.call(ctx, data)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmptyModelOutput
	}
	return text, nil
}
