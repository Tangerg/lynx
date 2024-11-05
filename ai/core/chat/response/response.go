package response

import (
	"errors"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/result"
	"github.com/Tangerg/lynx/ai/core/model"
)

type ChatResponse[M result.ChatResultMetadata] struct {
	metadata *ChatResponseMetadata
	results  []model.Result[*message.AssistantMessage, M]
}

func (c *ChatResponse[M]) Result() model.Result[*message.AssistantMessage, M] {
	if len(c.results) == 0 {
		return nil
	}
	return c.results[0]
}

func (c *ChatResponse[M]) Results() []model.Result[*message.AssistantMessage, M] {
	return c.results
}

func (c *ChatResponse[M]) Metadata() model.ResponseMetadata {
	return c.metadata
}

type ChatResponseBuilder[M result.ChatResultMetadata] struct {
	completion *ChatResponse[M]
}

func NewChatResponseBuilder[M result.ChatResultMetadata]() *ChatResponseBuilder[M] {
	return &ChatResponseBuilder[M]{
		completion: &ChatResponse[M]{
			results: make([]model.Result[*message.AssistantMessage, M], 0),
		},
	}
}

func (b *ChatResponseBuilder[M]) NewChatResultBuilder() *result.ChatResultBuilder[M] {
	return result.NewChatResultBuilder[M]()
}

func (b *ChatResponseBuilder[M]) WithChatResults(results ...*result.ChatResult[M]) *ChatResponseBuilder[M] {
	for _, res := range results {
		b.completion.results = append(b.completion.results, res)
	}
	return b
}

func (b *ChatResponseBuilder[M]) WithMetadata(metadata *ChatResponseMetadata) *ChatResponseBuilder[M] {
	b.completion.metadata = metadata
	return b
}

func (b *ChatResponseBuilder[M]) Build() (*ChatResponse[M], error) {
	//if b.completion.metadata == nil {
	//	return nil, errors.New("metadata is nil")
	//}
	if b.completion.results == nil || len(b.completion.results) == 0 {
		return nil, errors.New("results is nil")
	}
	return b.completion, nil
}
