package advisor

import (
	"github.com/Tangerg/lynx/ai/core/chat/client/advisor/api"
	"github.com/Tangerg/lynx/ai/core/chat/memory"
	"github.com/Tangerg/lynx/ai/core/chat/message"
	"github.com/Tangerg/lynx/ai/core/chat/metadata"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

func NewMessageChatMemory[O prompt.ChatOptions, M metadata.ChatGenerationMetadata](store memory.ChatMemory) *MessageChatMemory[O, M] {
	return &MessageChatMemory[O, M]{
		&ChatMemoryAdvisor[O, M, memory.ChatMemory]{
			store,
		},
	}
}

var _ api.RequestAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*MessageChatMemory[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)
var _ api.ResponseAdvisor[prompt.ChatOptions, metadata.ChatGenerationMetadata] = (*MessageChatMemory[prompt.ChatOptions, metadata.ChatGenerationMetadata])(nil)

type MessageChatMemory[O prompt.ChatOptions, M metadata.ChatGenerationMetadata] struct {
	*ChatMemoryAdvisor[O, M, memory.ChatMemory]
}

func (c *MessageChatMemory[O, M]) AdviseRequest(ctx *api.Context[O, M]) error {
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
		NewAdvisedRequestBuilder[O, M]().
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

func (c *MessageChatMemory[O, M]) AdviseStreamResponse(ctx *api.Context[O, M]) error {
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

func (c *MessageChatMemory[O, M]) AdviseCallResponse(ctx *api.Context[O, M]) error {
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
