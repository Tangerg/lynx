package request

import (
	"context"
	"errors"
)

var _ ChatRequestOptions = (*DefaultChatRequestOptions)(nil)

type DefaultChatRequestOptions struct {
	model           *string
	maxTokens       *int64
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int64
	topP            *float64
	streamFunc      func(ctx context.Context, chunk []byte) error
}

func (d *DefaultChatRequestOptions) StreamFunc() func(ctx context.Context, chunk []byte) error {
	return d.streamFunc
}
func (d *DefaultChatRequestOptions) Model() *string {
	return d.model
}

func (d *DefaultChatRequestOptions) MaxTokens() *int64 {
	return d.maxTokens
}

func (d *DefaultChatRequestOptions) PresencePenalty() *float64 {
	return d.presencePenalty
}

func (d *DefaultChatRequestOptions) StopSequences() []string {
	return d.stopSequences
}

func (d *DefaultChatRequestOptions) Temperature() *float64 {
	return d.temperature
}

func (d *DefaultChatRequestOptions) TopK() *int64 {
	return d.topK
}

func (d *DefaultChatRequestOptions) TopP() *float64 {
	return d.topP
}

func (d *DefaultChatRequestOptions) Clone() ChatRequestOptions {
	builder := NewDefaultChatOptionsBuilder()
	if d.model != nil {
		builder.WithModel(*d.model)
	}
	if d.maxTokens != nil {
		builder.WithMaxTokens(*d.maxTokens)
	}
	if d.presencePenalty != nil {
		builder.WithPresencePenalty(*d.presencePenalty)
	}
	if d.stopSequences != nil {
		builder.WithStopSequences(d.stopSequences)
	}
	if d.temperature != nil {
		builder.WithTemperature(*d.temperature)
	}
	if d.topK != nil {
		builder.WithTopK(*d.topK)
	}
	if d.topP != nil {
		builder.WithTopP(*d.topP)
	}

	cp, _ := builder.Build()
	return cp
}

type DefaultChatOptionsBuilder struct {
	options *DefaultChatRequestOptions
}

func NewDefaultChatOptionsBuilder() *DefaultChatOptionsBuilder {
	return &DefaultChatOptionsBuilder{
		options: &DefaultChatRequestOptions{},
	}
}

func (d *DefaultChatOptionsBuilder) WithModel(model string) *DefaultChatOptionsBuilder {
	d.options.model = &model
	return d
}
func (d *DefaultChatOptionsBuilder) WithMaxTokens(maxTokens int64) *DefaultChatOptionsBuilder {
	d.options.maxTokens = &maxTokens
	return d
}
func (d *DefaultChatOptionsBuilder) WithPresencePenalty(presencePenalty float64) *DefaultChatOptionsBuilder {
	d.options.presencePenalty = &presencePenalty
	return d
}
func (d *DefaultChatOptionsBuilder) WithStopSequences(stopSequences []string) *DefaultChatOptionsBuilder {
	d.options.stopSequences = stopSequences
	return d
}
func (d *DefaultChatOptionsBuilder) WithTemperature(temperature float64) *DefaultChatOptionsBuilder {
	d.options.temperature = &temperature
	return d
}
func (d *DefaultChatOptionsBuilder) WithTopK(topK int64) *DefaultChatOptionsBuilder {
	d.options.topK = &topK
	return d
}
func (d *DefaultChatOptionsBuilder) WithTopP(topP float64) *DefaultChatOptionsBuilder {
	d.options.topP = &topP
	return d
}
func (d *DefaultChatOptionsBuilder) WithStreamFunc(f func(ctx context.Context, chunk []byte) error) *DefaultChatOptionsBuilder {
	d.options.streamFunc = f
	return d
}
func (d *DefaultChatOptionsBuilder) Build() (*DefaultChatRequestOptions, error) {
	if d.options.model != nil {
		return nil, errors.New("model is required")
	}
	return d.options, nil
}
