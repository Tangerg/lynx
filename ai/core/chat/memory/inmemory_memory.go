package memory

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/core/message"
)

func NewInMemoryMemory() Memory {
	return &InMemoryMemory{
		conversations: make(map[string][]message.Message),
	}
}

type InMemoryMemory struct {
	conversations map[string][]message.Message
}

func (i *InMemoryMemory) Add(ctx context.Context, conversationId string, messages ...message.Message) error {
	if i.conversations[conversationId] == nil {
		i.conversations[conversationId] = make([]message.Message, 0)
	}
	i.conversations[conversationId] = append(i.conversations[conversationId], messages...)
	return nil
}

func (i *InMemoryMemory) Get(ctx context.Context, conversationId string, lastN int) ([]message.Message, error) {
	if i.conversations[conversationId] == nil {
		return nil, fmt.Errorf("conversation %s not found", conversationId)
	}
	return i.conversations[conversationId][lastN:], nil
}

func (i *InMemoryMemory) Clear(ctx context.Context, conversationId string) error {
	if i.conversations[conversationId] == nil {
		return nil
	}
	i.conversations[conversationId] = make([]message.Message, 0)
	return nil
}
