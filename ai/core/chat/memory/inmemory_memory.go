package memory

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/message"
)

func NewInMemoryChatMemory() ChatMemory {
	return &InMemoryChatMemory{
		conversations: make(map[string][]message.Message),
	}
}

type InMemoryChatMemory struct {
	conversations map[string][]message.Message
}

func (i *InMemoryChatMemory) Add(ctx context.Context, conversationId string, messages ...message.Message) error {
	if i.conversations[conversationId] == nil {
		i.conversations[conversationId] = make([]message.Message, 0)
	}
	i.conversations[conversationId] = append(i.conversations[conversationId], messages...)
	return nil
}

func (i *InMemoryChatMemory) Get(ctx context.Context, conversationId string, lastN int) ([]message.Message, error) {
	if i.conversations[conversationId] == nil {
		return nil, fmt.Errorf("conversation %s not found", conversationId)
	}
	return i.conversations[conversationId][lastN:], nil
}

func (i *InMemoryChatMemory) Clear(ctx context.Context, conversationId string) error {
	if i.conversations[conversationId] == nil {
		return nil
	}
	i.conversations[conversationId] = make([]message.Message, 0)
	return nil
}
