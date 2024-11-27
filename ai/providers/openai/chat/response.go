package chat

import (
	"github.com/Tangerg/lynx/ai/core/chat/response"
	"github.com/sashabaranov/go-openai"
	"time"
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

var _ response.RateLimit = (*OpenAIRateLimit)(nil)

type OpenAIRateLimit struct {
	requestsLimit     int64
	requestsRemaining int64
	requestsReset     time.Duration
	tokensLimit       int64
	tokensRemaining   int64
	tokensReset       time.Duration
}

func NewOpenAIRateLimit() *OpenAIRateLimit {
	return &OpenAIRateLimit{}
}

func (o *OpenAIRateLimit) SetRequestsLimit(requestsLimit int64) *OpenAIRateLimit {
	o.requestsLimit = requestsLimit
	return o
}

func (o *OpenAIRateLimit) SetRequestsRemaining(requestsRemaining int64) *OpenAIRateLimit {
	o.requestsRemaining = requestsRemaining
	return o
}

func (o *OpenAIRateLimit) SetRequestsReset(requestsReset openai.ResetTime) *OpenAIRateLimit {
	duration, _ := time.ParseDuration(requestsReset.String())
	o.requestsReset = duration
	return o
}

func (o *OpenAIRateLimit) SetTokensLimit(tokensLimit int64) *OpenAIRateLimit {
	o.tokensLimit = tokensLimit
	return o
}

func (o *OpenAIRateLimit) SetTokensRemaining(tokensRemaining int64) *OpenAIRateLimit {
	o.tokensRemaining = tokensRemaining
	return o
}

func (o *OpenAIRateLimit) SetTokensReset(tokensReset openai.ResetTime) *OpenAIRateLimit {
	duration, _ := time.ParseDuration(tokensReset.String())
	o.tokensReset = duration
	return o
}

func (o *OpenAIRateLimit) RequestsLimit() int64 {
	return o.requestsLimit
}

func (o *OpenAIRateLimit) RequestsRemaining() int64 {
	return o.requestsRemaining
}

func (o *OpenAIRateLimit) RequestsReset() time.Duration {
	return o.requestsReset
}

func (o *OpenAIRateLimit) TokensLimit() int64 {
	return o.tokensLimit
}

func (o *OpenAIRateLimit) TokensRemaining() int64 {
	return o.tokensRemaining
}

func (o *OpenAIRateLimit) TokensReset() time.Duration {
	return o.tokensReset
}

type OpenAIChatResponse = response.ChatResponse[*OpenAIChatResultMetadata]

func newOpenAIChatResponseBuilder() *response.ChatResponseBuilder[*OpenAIChatResultMetadata] {
	return response.NewChatResponseBuilder[*OpenAIChatResultMetadata]()
}
