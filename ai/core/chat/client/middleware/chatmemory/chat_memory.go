package chatmemory

import (
	"context"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/core/chat/client/middleware"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/result"
)

const (
	ConversationIdKey = "chat_memory_conversation_id"
	RetrieveSizeKey   = "chat_memory_response_size"
)

const (
	defaultChatMemoryResponseSize = 100
)

type Config struct {
	ConversationIdKey string `json:"ConversationIdKey" yaml:"ConversationIdKey"`
	RetrieveSizeKey   string `json:"RetrieveSizeKey" yaml:"RetrieveSizeKey"`
	Store             memory.ChatMemory
}

func newChatMemory[O request.ChatRequestOptions, M result.ChatResultMetadata](conf *Config) *chatMemory[O, M] {
	if conf.ConversationIdKey == "" {
		conf.ConversationIdKey = ConversationIdKey
	}
	if conf.RetrieveSizeKey == "" {
		conf.RetrieveSizeKey = RetrieveSizeKey
	}
	if conf.Store == nil {
		panic("the chat memory store can not nil")
	}

	return &chatMemory[O, M]{
		conversationIdKey: conf.ConversationIdKey,
		retrieveSizeKey:   conf.RetrieveSizeKey,
		store:             conf.Store,
	}
}

type chatMemory[O request.ChatRequestOptions, M result.ChatResultMetadata] struct {
	conversationIdKey string
	retrieveSizeKey   string
	store             memory.ChatMemory
}

func (c *chatMemory[O, M]) getConversationId(m map[string]any) string {
	id, ok := m[c.conversationIdKey]
	if !ok {
		return ""
	}
	return cast.ToString(id)
}

func (c *chatMemory[O, M]) getChatMemoryRetrieveSize(m map[string]any) int {
	size, ok := m[c.retrieveSizeKey]
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
	for _, r := range ctx.Response.Results() {
		msgs = append(msgs, r.Output())
	}
	return c.saveMessages(ctx.Context(), ctx.Request.UserParams, msgs...)
}

func (c *chatMemory[O, M]) do(ctx *middleware.Context[O, M]) error {
	err := c.beforeRequest(ctx)
	if err != nil {
		return err
	}
	err = ctx.Next()
	if err != nil {
		return err
	}
	return c.afterRequest(ctx)
}

func New[O request.ChatRequestOptions, M result.ChatResultMetadata](conf *Config) middleware.Middleware[O, M] {
	return newChatMemory[O, M](conf).do
}
