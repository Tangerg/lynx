package chat

import (
	"github.com/Tangerg/lynx/ai/core/chat/prompt"
)

type OpenaiChatOptions struct {
	model           *string
	maxTokens       *int64
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int64
	topP            *float64
}

func (o *OpenaiChatOptions) UseStream() bool {
	return false
}

func (o *OpenaiChatOptions) Model() *string {
	return o.model
}

func (o *OpenaiChatOptions) MaxTokens() *int64 {
	return o.maxTokens
}

func (o *OpenaiChatOptions) PresencePenalty() *float64 {
	return o.presencePenalty
}

func (o *OpenaiChatOptions) StopSequences() []string {
	return o.stopSequences
}

func (o *OpenaiChatOptions) Temperature() *float64 {
	return o.temperature
}

func (o *OpenaiChatOptions) TopK() *int64 {
	return o.topK
}

func (o *OpenaiChatOptions) TopP() *float64 {
	return o.topP
}

func (o *OpenaiChatOptions) Copy() prompt.Options {
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
	return builder.Build()
}

type OpenaiChatOptionsBuilder struct {
	options *OpenaiChatOptions
}

func NewOpenaiChatOptionsBuilder() *OpenaiChatOptionsBuilder {
	return &OpenaiChatOptionsBuilder{
		options: &OpenaiChatOptions{},
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
func (o *OpenaiChatOptionsBuilder) Build() *OpenaiChatOptions {
	return o.options
}
