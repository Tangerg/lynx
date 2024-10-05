package completion

import (
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/model"
)

var _ model.Response[*message.AssistantMessage, metadata.ChatGenerationMetadata] = (*ChatCompletion[metadata.ChatGenerationMetadata])(nil)

type ChatCompletion[RM metadata.ChatGenerationMetadata] struct {
	metadata *metadata.ChatCompletionMetadata
	results  []model.Result[*message.AssistantMessage, RM]
}

func (c *ChatCompletion[RM]) Result() model.Result[*message.AssistantMessage, RM] {
	if len(c.results) == 0 {
		return nil
	}
	return c.results[0]
}

func (c *ChatCompletion[RM]) Results() []model.Result[*message.AssistantMessage, RM] {
	return c.results
}

func (c *ChatCompletion[RM]) Metadata() model.ResponseMetadata {
	return c.metadata
}

type ChatCompletionBuilder[RM metadata.ChatGenerationMetadata] struct {
	completion *ChatCompletion[RM]
}

func NewChatCompletionBuilder[RM metadata.ChatGenerationMetadata]() *ChatCompletionBuilder[RM] {
	return &ChatCompletionBuilder[RM]{
		completion: &ChatCompletion[RM]{
			results: make([]model.Result[*message.AssistantMessage, RM], 0),
		},
	}
}

func (b *ChatCompletionBuilder[RM]) NewChatGenerationBuilder() *ChatGenerationBuilder[RM] {
	return &ChatGenerationBuilder[RM]{
		result: &ChatGeneration[RM]{},
	}
}

func (b *ChatCompletionBuilder[RM]) WithChatGenerations(gens ...*ChatGeneration[RM]) *ChatCompletionBuilder[RM] {
	for _, gen := range gens {
		b.completion.results = append(b.completion.results, gen)
	}
	return b
}

func (b *ChatCompletionBuilder[RM]) WithMetadata(metadata *metadata.ChatCompletionMetadata) *ChatCompletionBuilder[RM] {
	b.completion.metadata = metadata
	return b
}

func (b *ChatCompletionBuilder[RM]) Build() (*ChatCompletion[RM], error) {
	//if b.completion.metadata == nil {
	//	return nil, errors.New("metadata is nil")
	//}
	if b.completion.results == nil || len(b.completion.results) == 0 {
		return nil, errors.New("results is nil")
	}
	return b.completion, nil
}
