package chat

import "github.com/Tangerg/lynx/ai/core/chat/response"

type OpenAIChatResponse = response.ChatResponse[*OpenAIChatResultMetadata]

func newOpenAIChatResponseBuilder() *response.ChatResponseBuilder[*OpenAIChatResultMetadata] {
	return response.NewChatResponseBuilder[*OpenAIChatResultMetadata]()
}
