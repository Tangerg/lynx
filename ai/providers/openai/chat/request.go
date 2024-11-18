package chat

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
)

type OpenAIChatRequest = request.ChatRequest[*OpenAIChatRequestOptions]

func newOpenAIChatRequestBuilder() *request.ChatRequestBuilder[*OpenAIChatRequestOptions] {
	return request.NewChatRequestBuilder[*OpenAIChatRequestOptions]()
}
