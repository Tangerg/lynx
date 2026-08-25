package rag

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	promptVariableContext = "Context"
	promptVariableHistory = "History"
	promptVariableNumber  = "Number"
	promptVariableQuery   = "Query"
	promptVariableTarget  = "Target"
)

// ErrEmptyModelOutput reports a successful model call that produced no usable
// query text. A requested transform or expansion must not silently become an
// identity operation.
var ErrEmptyModelOutput = errors.New("rag: model returned empty query text")

type modelPrompt struct {
	generation chatclient.Generation[string]
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

func (p modelPrompt) call(ctx context.Context, data any) (string, error) {
	message, err := p.template.UserMessage(data)
	if err != nil {
		return "", err
	}
	text, err := p.generation.Call(ctx, &chat.Request{Messages: []chat.Message{message}})
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ErrEmptyModelOutput
	}
	return text, nil
}
