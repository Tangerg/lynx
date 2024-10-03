package advisor

import (
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
)

const (
	ChatMemoryConversationIdKey = "chat_memory_conversation_id"
	ChatMemoryRetrieveSizeKey   = "chat_memory_response_size"
)

const (
	defaultChatMemoryResponseSize = 100
)

var _ api.RequestAdvisor = (*ChatMemoryAdvisor[any])(nil)
var _ api.ResponseAdvisor = (*ChatMemoryAdvisor[any])(nil)

type ChatMemoryAdvisor[T any] struct {
	chatMemoryStore T
}

func (c *ChatMemoryAdvisor[T]) Name() string {
	return "ChatMemoryAdvisor"
}

func (c *ChatMemoryAdvisor[T]) AdviseRequest(_ *api.Context) error {
	return nil
}

func (c *ChatMemoryAdvisor[T]) AdviseStreamResponse(_ *api.Context) error {
	return nil
}

func (c *ChatMemoryAdvisor[T]) AdviseCallResponse(_ *api.Context) error {
	return nil
}

func (c *ChatMemoryAdvisor[T]) getChatMemoryStore() T {
	return c.chatMemoryStore
}

func (c *ChatMemoryAdvisor[T]) doGetConversationId(m map[string]any) string {
	id, ok := m[ChatMemoryConversationIdKey]
	if !ok {
		return ""
	}
	return cast.ToString(id)
}

func (c *ChatMemoryAdvisor[T]) doGetChatMemoryRetrieveSize(m map[string]any) int {
	size, ok := m[ChatMemoryRetrieveSizeKey]
	if !ok {
		return defaultChatMemoryResponseSize
	}
	return cast.ToInt(size)
}
