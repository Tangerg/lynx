package chat

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

var _ prompt.ChatOptions = (*OpenAIChatOptions)(nil)

type OpenAIChatOptions struct {
	model                *string
	maxTokens            *int64
	presencePenalty      *float64
	stopSequences        []string
	temperature          *float64
	topK                 *int64
	topP                 *float64
	n                    int
	streamChunkFunc      func(ctx context.Context, chunk string) error
	streamCompletionFunc func(ctx context.Context, completion OpenAIChatCompletion) error
}

func (o *OpenAIChatOptions) Model() *string {
	return o.model
}

func (o *OpenAIChatOptions) MaxTokens() *int64 {
	return o.maxTokens
}

func (o *OpenAIChatOptions) PresencePenalty() *float64 {
	return o.presencePenalty
}

func (o *OpenAIChatOptions) StopSequences() []string {
	return o.stopSequences
}

func (o *OpenAIChatOptions) Temperature() *float64 {
	return o.temperature
}

func (o *OpenAIChatOptions) TopK() *int64 {
	return o.topK
}

func (o *OpenAIChatOptions) TopP() *float64 {
	return o.topP
}

func (o *OpenAIChatOptions) N() int {
	if o.n < 1 {
		o.n = 1
	}
	return o.n
}

func (o *OpenAIChatOptions) StreamChunkFunc() func(ctx context.Context, chunk string) error {
	return o.streamChunkFunc
}

func (o *OpenAIChatOptions) StreamCompletionFunc() func(ctx context.Context, completion OpenAIChatCompletion) error {
	return o.streamCompletionFunc
}

func (o *OpenAIChatOptions) Clone() prompt.ChatOptions {
	builder := NewOpenAIChatOptionsBuilder()
	if o.model != nil {
		builder.WithModel(*o.model)
	}
	if o.maxTokens != nil {
		builder.WithMaxTokens(*o.maxTokens)
	}
	if o.presencePenalty != nil {
		builder.WithPresencePenalty(*o.presencePenalty)
	}
	if o.stopSequences != nil {
		builder.WithStopSequences(o.stopSequences)
	}
	if o.temperature != nil {
		builder.WithTemperature(*o.temperature)
	}
	if o.topK != nil {
		builder.WithTopK(*o.topK)
	}
	if o.topP != nil {
		builder.WithTopP(*o.topP)
	}
	if o.streamChunkFunc != nil {
		builder.WithStreamChunkFunc(o.streamChunkFunc)
	}
	cp, _ := builder.Build()
	return cp
}

type OpenAIChatOptionsBuilder struct {
	options *OpenAIChatOptions
}

func NewOpenAIChatOptionsBuilder() *OpenAIChatOptionsBuilder {
	return &OpenAIChatOptionsBuilder{
		options: &OpenAIChatOptions{},
	}
}

func (o *OpenAIChatOptionsBuilder) WithModel(model string) *OpenAIChatOptionsBuilder {
	o.options.model = &model
	return o
}
func (o *OpenAIChatOptionsBuilder) WithMaxTokens(maxTokens int64) *OpenAIChatOptionsBuilder {
	o.options.maxTokens = &maxTokens
	return o
}
func (o *OpenAIChatOptionsBuilder) WithPresencePenalty(presencePenalty float64) *OpenAIChatOptionsBuilder {
	o.options.presencePenalty = &presencePenalty
	return o
}
func (o *OpenAIChatOptionsBuilder) WithStopSequences(stopSequences []string) *OpenAIChatOptionsBuilder {
	o.options.stopSequences = stopSequences
	return o
}
func (o *OpenAIChatOptionsBuilder) WithTemperature(temperature float64) *OpenAIChatOptionsBuilder {
	o.options.temperature = &temperature
	return o
}
func (o *OpenAIChatOptionsBuilder) WithTopK(topK int64) *OpenAIChatOptionsBuilder {
	o.options.topK = &topK
	return o
}
func (o *OpenAIChatOptionsBuilder) WithTopP(topP float64) *OpenAIChatOptionsBuilder {
	o.options.topP = &topP
	return o
}
func (o *OpenAIChatOptionsBuilder) WithN(n int) *OpenAIChatOptionsBuilder {
	o.options.n = n
	return o
}
func (o *OpenAIChatOptionsBuilder) WithStreamChunkFunc(f func(ctx context.Context, chunk string) error) *OpenAIChatOptionsBuilder {
	o.options.streamChunkFunc = f
	return o
}
func (o *OpenAIChatOptionsBuilder) WithStreamCompletionFunc(f func(ctx context.Context, completion OpenAIChatCompletion) error) *OpenAIChatOptionsBuilder {
	o.options.streamCompletionFunc = f
	return o
}
func (o *OpenAIChatOptionsBuilder) Build() (*OpenAIChatOptions, error) {
	if o.options.model == nil {
		return nil, errors.New("model is required")
	}
	return o.options, nil
}
