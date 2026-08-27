package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/chatclient"
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

type modelPrompt struct {
	generation chatclient.Generation[string]
	template   *chatclient.Template
}

type structuredModelPrompt[T any] struct {
	generation chatclient.Generation[T]
	template   *chatclient.Template
}

func newModelPrompt(
	model chat.Model,
	template *chatclient.Template,
	fallback string,
	required ...string,
) (modelPrompt, error) {
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		return modelPrompt{}, err
	}
	template, err = resolvePromptTemplate(template, fallback, required...)
	if err != nil {
		return modelPrompt{}, err
	}
	return modelPrompt{generation: client.Output(chatclient.Text()), template: template}, nil
}

func newStructuredModelPrompt[T any](
	model chat.Model,
	format chatclient.OutputFormat[T],
	template *chatclient.Template,
	fallback string,
	required ...string,
) (structuredModelPrompt[T], error) {
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		return structuredModelPrompt[T]{}, err
	}
	template, err = resolvePromptTemplate(template, fallback, required...)
	if err != nil {
		return structuredModelPrompt[T]{}, err
	}
	return structuredModelPrompt[T]{generation: client.Output(format), template: template}, nil
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

func (m modelPrompt) call(ctx context.Context, data any) (string, error) {
	message, err := m.template.UserMessage(data)
	if err != nil {
		return "", err
	}
	text, err := m.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmptyModelOutput
	}
	return text, nil
}

func (m structuredModelPrompt[T]) call(ctx context.Context, data any) (T, error) {
	var zero T
	message, err := m.template.UserMessage(data)
	if err != nil {
		return zero, err
	}
	return m.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
}
