package advisor

import (
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

const (
	ChatMemoryConversationIdKey = "chat_memory_conversation_id"
	ChatMemoryRetrieveSizeKey   = "chat_memory_response_size"
)

const (
	defaultChatMemoryResponseSize = 100
)

var _ api.RequestAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*ChatMemoryAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata, any])(nil)
var _ api.ResponseAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*ChatMemoryAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata, any])(nil)

type ChatMemoryAdvisor[O prompt.ChatOptions, M metadata.ChatGenerationMetadata, T any] struct {
	chatMemoryStore T
}

func (c *ChatMemoryAdvisor[O, M, T]) Name() string {
	return "ChatMemoryAdvisor"
}

func (c *ChatMemoryAdvisor[O, M, T]) AdviseRequest(_ *api.Context[O, M]) error {
	return nil
}

func (c *ChatMemoryAdvisor[O, M, T]) AdviseStreamResponse(_ *api.Context[O, M]) error {
	return nil
}

func (c *ChatMemoryAdvisor[O, M, T]) AdviseCallResponse(_ *api.Context[O, M]) error {
	return nil
}

func (c *ChatMemoryAdvisor[O, M, T]) getChatMemoryStore() T {
	return c.chatMemoryStore
}

func (c *ChatMemoryAdvisor[O, M, T]) doGetConversationId(m map[string]any) string {
	id, ok := m[ChatMemoryConversationIdKey]
	if !ok {
		return ""
	}
	return cast.ToString(id)
}

func (c *ChatMemoryAdvisor[O, M, T]) doGetChatMemoryRetrieveSize(m map[string]any) int {
	size, ok := m[ChatMemoryRetrieveSizeKey]
	if !ok {
		return defaultChatMemoryResponseSize
	}
	return cast.ToInt(size)
}
