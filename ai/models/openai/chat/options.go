package chat

import (
	"context"
	"errors"
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

var _ prompt.Options = (*OpenAIChatOptions)(nil)

type OpenAIChatOptions struct {
	model           *string
	maxTokens       *int64
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int64
	topP            *float64
	streamFunc      func(ctx context.Context, chunk []byte) error
}

func (o *OpenAIChatOptions) StreamFunc() func(ctx context.Context, chunk []byte) error {
	return o.streamFunc
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

func (o *OpenAIChatOptions) Copy() prompt.Options {
	builder := NewOpenaiChatOptionsBuilder()
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
	if o.streamFunc != nil {
		builder.WithStreamFunc(o.streamFunc)
	}
	cp, _ := builder.Build()
	return cp
}

type OpenaiChatOptionsBuilder struct {
	options *OpenAIChatOptions
}

func NewOpenaiChatOptionsBuilder() *OpenaiChatOptionsBuilder {
	return &OpenaiChatOptionsBuilder{
		options: &OpenAIChatOptions{},
	}
}

func (o *OpenaiChatOptionsBuilder) WithModel(model string) *OpenaiChatOptionsBuilder {
	o.options.model = &model
	return o
}
func (o *OpenaiChatOptionsBuilder) WithMaxTokens(maxTokens int64) *OpenaiChatOptionsBuilder {
	o.options.maxTokens = &maxTokens
	return o
}
func (o *OpenaiChatOptionsBuilder) WithPresencePenalty(presencePenalty float64) *OpenaiChatOptionsBuilder {
	o.options.presencePenalty = &presencePenalty
	return o
}
func (o *OpenaiChatOptionsBuilder) WithStopSequences(stopSequences []string) *OpenaiChatOptionsBuilder {
	o.options.stopSequences = stopSequences
	return o
}
func (o *OpenaiChatOptionsBuilder) WithTemperature(temperature float64) *OpenaiChatOptionsBuilder {
	o.options.temperature = &temperature
	return o
}
func (o *OpenaiChatOptionsBuilder) WithTopK(topK int64) *OpenaiChatOptionsBuilder {
	o.options.topK = &topK
	return o
}
func (o *OpenaiChatOptionsBuilder) WithTopP(topP float64) *OpenaiChatOptionsBuilder {
	o.options.topP = &topP
	return o
}
func (o *OpenaiChatOptionsBuilder) WithStreamFunc(f func(ctx context.Context, chunk []byte) error) *OpenaiChatOptionsBuilder {
	o.options.streamFunc = f
	return o
}
func (o *OpenaiChatOptionsBuilder) Build() (*OpenAIChatOptions, error) {
	if o.options.model == nil {
		return nil, errors.New("model is required")
	}
	return o.options, nil
}
