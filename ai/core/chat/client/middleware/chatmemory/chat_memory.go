package chatmemory

import (
	"context"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/core/chat/message"
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

type Config struct {
	ChatMemoryConversationIdKey string `json:"ChatMemoryConversationIdKey" yaml:"ChatMemoryConversationIdKey"`
	ChatMemoryRetrieveSizeKey   string `json:"ChatMemoryRetrieveSizeKey" yaml:"ChatMemoryRetrieveSizeKey"`
	Store                       memory.ChatMemory
}

func newChatMemory[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](conf *Config) *chatMemory[O, M] {
	if conf.ChatMemoryConversationIdKey == "" {
		conf.ChatMemoryConversationIdKey = ChatMemoryConversationIdKey
	}
	if conf.ChatMemoryRetrieveSizeKey == "" {
		conf.ChatMemoryRetrieveSizeKey = ChatMemoryRetrieveSizeKey
	}
	if conf.Store == nil {
		panic("the chat memory store can not nil")
	}

	return &chatMemory[O, M]{
		chatMemoryConversationIdKey: conf.ChatMemoryConversationIdKey,
		chatMemoryRetrieveSizeKey:   conf.ChatMemoryRetrieveSizeKey,
		store:                       conf.Store,
	}
}

type chatMemory[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	chatMemoryConversationIdKey string
	chatMemoryRetrieveSizeKey   string
	store                       memory.ChatMemory
}

func (c *chatMemory[O, M]) getConversationId(m map[string]any) string {
	id, ok := m[c.chatMemoryConversationIdKey]
	if !ok {
		return ""
	}
	return cast.ToString(id)
}

func (c *chatMemory[O, M]) getChatMemoryRetrieveSize(m map[string]any) int {
	size, ok := m[c.chatMemoryRetrieveSizeKey]
	if !ok {
		return defaultChatMemoryResponseSize
	}
	return cast.ToInt(size)
}

func (c *chatMemory[O, M]) getMessages(ctx context.Context, m map[string]any) ([]message.ChatMessage, error) {
	cid := c.getConversationId(m)
	size := c.getChatMemoryRetrieveSize(m)
	return c.store.Get(ctx, cid, size)
}

func (c *chatMemory[O, M]) saveMessages(ctx context.Context, m map[string]any, msgs ...message.ChatMessage) error {
	cid := c.getConversationId(m)
	return c.store.Add(ctx, cid, msgs...)
}

func (c *chatMemory[O, M]) beforeRequest(ctx *middleware.Context[O, M]) error {
	msgs, err := c.getMessages(ctx.Context(), ctx.Request.UserParams)
	if err != nil {
		return err
	}
	err = c.saveMessages(
		ctx.Context(),
		ctx.Request.UserParams,
		message.NewUserMessage(ctx.Request.UserText, nil, ctx.Request.UserMedia...),
	)
	if err != nil {
		return err
	}
	ctx.Request.AddMessage(msgs...)
	return nil
}

func (c *chatMemory[O, M]) afterRequest(ctx *middleware.Context[O, M]) error {
	msgs := make([]message.ChatMessage, 0, len(ctx.Response.Results()))
	for _, result := range ctx.Response.Results() {
		msgs = append(msgs, result.Output())
	}
	return c.saveMessages(ctx.Context(), ctx.Request.UserParams, msgs...)
}

func New[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](conf *Config) middleware.Middleware[O, M] {
	cm := newChatMemory[O, M](conf)
	return func(ctx *middleware.Context[O, M]) error {
		err := cm.beforeRequest(ctx)
		if err != nil {
			return err
		}
		err = ctx.Next()
		if err != nil {
			return err
		}
		return cm.afterRequest(ctx)
	}
}
