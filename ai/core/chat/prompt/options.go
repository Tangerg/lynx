package prompt

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/ai/core/model"
)

// ChatOptions interface defines a set of methods for configuring model generation options.
type ChatOptions interface {
	model.Options

	// Model returns a pointer to a string representing the name of the model to be used.
	Model() *string

	// MaxTokens returns a pointer to an int64 representing the maximum number of tokens to generate.
	MaxTokens() *int64

	// PresencePenalty returns a pointer to a float64 used to set the penalty for the presence of certain tokens in the generated text.
	PresencePenalty() *float64

	// StopSequences returns a slice of strings containing the sequences that will stop the text generation.
	StopSequences() []string

	// Temperature returns a pointer to a float64 used to set the randomness of the text generation.
	Temperature() *float64

	// TopK returns a pointer to an int64 used to set the top-k sampling parameter for text generation.
	TopK() *int64

	// TopP returns a pointer to a float64 used to set the nucleus sampling parameter for text generation.
	TopP() *float64

	// Copy returns a new instance of ChatOptions, copying the current configuration.
	Copy() ChatOptions
}

type DefaultChatOptions struct {
	model           *string
	maxTokens       *int64
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int64
	topP            *float64
	streamFunc      func(ctx context.Context, chunk []byte) error
}

func (d *DefaultChatOptions) StreamFunc() func(ctx context.Context, chunk []byte) error {
	return d.streamFunc
}
func (d *DefaultChatOptions) Model() *string {
	return d.model
}

func (d *DefaultChatOptions) MaxTokens() *int64 {
	return d.maxTokens
}

func (d *DefaultChatOptions) PresencePenalty() *float64 {
	return d.presencePenalty
}

func (d *DefaultChatOptions) StopSequences() []string {
	return d.stopSequences
}

func (d *DefaultChatOptions) Temperature() *float64 {
	return d.temperature
}

func (d *DefaultChatOptions) TopK() *int64 {
	return d.topK
}

func (d *DefaultChatOptions) TopP() *float64 {
	return d.topP
}

func (d *DefaultChatOptions) Copy() ChatOptions {
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
	options *DefaultChatOptions
}

func NewDefaultChatOptionsBuilder() *DefaultChatOptionsBuilder {
	return &DefaultChatOptionsBuilder{
		options: &DefaultChatOptions{},
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
func (d *DefaultChatOptionsBuilder) Build() (ChatOptions, error) {
	if d.options.model != nil {
		return nil, errors.New("model is required")
	}
	return d.options, nil
}
