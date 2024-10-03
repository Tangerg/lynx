package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/core/chat/message"
)

type MessageChatMemory struct {
	*ChatMemoryAdvisor[memory.ChatMemory]
}

func NewMessageChatMemory(store memory.ChatMemory) *MessageChatMemory {
	return &MessageChatMemory{
		&ChatMemoryAdvisor[memory.ChatMemory]{
			store,
		},
	}
}

func (c *MessageChatMemory) AdviseRequest(ctx *api.Context) error {
	cid := c.doGetConversationId(ctx.Params())
	size := c.doGetChatMemoryRetrieveSize(ctx.Params())

	msgs, err := c.
		getChatMemoryStore().
		Get(ctx.Context(), cid, size)
	if err != nil {
		return err
	}

	reqMsgs := ctx.Request.Messages()
	reqMsgs = append(ctx.Request.Messages(), msgs...)
	ctx.Request = api.
		NewAdvisedRequestBuilder().
		FromAdvisedRequest(ctx.Request).
		WithMessages(reqMsgs...).
		Build()

	return c.
		getChatMemoryStore().
		Add(
			ctx.Context(),
			cid,
			message.NewUserMessage(ctx.Request.UserText()),
		)
}

func (c *MessageChatMemory) AdviseStreamResponse(ctx *api.Context) error {
	msgs := make([]message.ChatMessage, 0)
	for _, result := range ctx.Response.Results() {
		msgs = append(msgs, result.Output())
	}
	return c.
		getChatMemoryStore().
		Add(
			ctx.Context(),
			c.doGetConversationId(ctx.Params()),
			msgs...,
		)
}

func (c *MessageChatMemory) AdviseCallResponse(ctx *api.Context) error {
	msgs := make([]message.ChatMessage, 0)
	for _, result := range ctx.Response.Results() {
		msgs = append(msgs, result.Output())
	}
	return c.
		getChatMemoryStore().
		Add(
			ctx.Context(),
			c.doGetConversationId(ctx.Params()),
			msgs...,
		)
}
