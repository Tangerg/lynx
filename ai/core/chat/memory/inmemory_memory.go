package memory

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/ai/core/chat/message"
)

func NewInMemoryChatMemory() ChatMemory {
	return &InMemoryChatMemory{
		conversations: make(map[string][]message.ChatMessage),
	}
}

type InMemoryChatMemory struct {
	conversations map[string][]message.ChatMessage
}

func (i *InMemoryChatMemory) Add(_ context.Context, conversationId string, messages ...message.ChatMessage) error {
	if i.conversations[conversationId] == nil {
		i.conversations[conversationId] = make([]message.ChatMessage, 0)
	}
	i.conversations[conversationId] = append(i.conversations[conversationId], messages...)
	return nil
}

func (i *InMemoryChatMemory) Get(_ context.Context, conversationId string, lastN int) ([]message.ChatMessage, error) {
	if i.conversations[conversationId] == nil {
		return nil, fmt.Errorf("conversation %s not found", conversationId)
	}
	return i.conversations[conversationId][lastN:], nil
}

func (i *InMemoryChatMemory) Clear(_ context.Context, conversationId string) error {
	if i.conversations[conversationId] == nil {
		return nil
	}
	i.conversations[conversationId] = make([]message.ChatMessage, 0)
	return nil
}
