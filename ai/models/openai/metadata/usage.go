package metadata

import (
	chatMetadata "github.com/Tangerg/lynx/ai/core/chat/metadata"
)

type OpenAIUsage struct {
	promptTokens     int64
	completionTokens int64
	reasoningTokens  int64
	totalTokens      int64
}

func NewOpenAIUsage() chatMetadata.Usage {
	return &OpenAIUsage{}
}

func (o *OpenAIUsage) IncrPromptTokens(tokens int64) {
	o.promptTokens = o.promptTokens + tokens
}

func (o *OpenAIUsage) IncrCompletionTokens(tokens int64) {
	o.completionTokens = o.completionTokens + tokens
}

func (o *OpenAIUsage) IncrReasoningTokens(tokens int64) {
	o.reasoningTokens = o.reasoningTokens + tokens
}

func (o *OpenAIUsage) IncrTotalTokens(tokens int64) {
	o.totalTokens = o.totalTokens + tokens
}

func (o *OpenAIUsage) PromptTokens() int64 {
	return o.promptTokens
}

func (o *OpenAIUsage) CompletionTokens() int64 {
	return o.completionTokens
}

func (o *OpenAIUsage) ReasoningTokens() int64 {
	return o.reasoningTokens
}

func (o *OpenAIUsage) TotalTokens() int64 {
	if o.totalTokens == 0 {
		return o.promptTokens + o.completionTokens
	}
	return o.totalTokens
}
