package chat

import (
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/chat/response"
)

var _ response.Usage = (*OpenAIUsage)(nil)

type OpenAIUsage struct {
	promptTokens     int64
	completionTokens int64
	reasoningTokens  int64
	totalTokens      int64
}

func NewOpenAIUsage() *OpenAIUsage {
	return &OpenAIUsage{}
}

func (o *OpenAIUsage) IncrPromptTokens(tokens int64) *OpenAIUsage {
	o.promptTokens = o.promptTokens + tokens
	return o
}

func (o *OpenAIUsage) IncrCompletionTokens(tokens int64) *OpenAIUsage {
	o.completionTokens = o.completionTokens + tokens
	return o
}

func (o *OpenAIUsage) IncrReasoningTokens(tokens int64) *OpenAIUsage {
	o.reasoningTokens = o.reasoningTokens + tokens
	return o
}

func (o *OpenAIUsage) IncrTotalTokens(tokens int64) *OpenAIUsage {
	o.totalTokens = o.totalTokens + tokens
	return o
}

func (o *OpenAIUsage) PromptTokens() int64 {
	return o.promptTokens
}

func (o *OpenAIUsage) CompletionTokens() int64 {
	return o.completionTokens + o.reasoningTokens
}

func (o *OpenAIUsage) ReasoningTokens() int64 {
	return o.reasoningTokens
}

func (o *OpenAIUsage) TotalTokens() int64 {
	if o.totalTokens == 0 {
		return o.PromptTokens() + o.CompletionTokens()
	}
	return o.totalTokens
}

type OpenAIChatRequest = request.ChatRequest[*OpenAIChatRequestOptions]

func newOpenAIChatRequestBuilder() *request.ChatRequestBuilder[*OpenAIChatRequestOptions] {
	return request.NewChatRequestBuilder[*OpenAIChatRequestOptions]()
}
