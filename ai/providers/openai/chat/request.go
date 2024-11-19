package chat

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	baseModel "github.com/Tangerg/lynx/ai/core/model"
)

type OpenAIChatRequest = request.ChatRequest[*OpenAIChatRequestOptions]

func newOpenAIChatRequestBuilder() *request.ChatRequestBuilder[*OpenAIChatRequestOptions] {
	return request.NewChatRequestBuilder[*OpenAIChatRequestOptions]()
}

type OpenAIChatStreamChunkHandler = baseModel.StreamChunkHandler[*OpenAIChatResponse]
