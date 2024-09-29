package prompt

import "github.com/Tangerg/lynx/ai/core/model"

// Options interface defines a set of methods for configuring model generation options.
type Options interface {
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

	// Copy returns a new instance of Options, copying the current configuration.
	Copy() Options
}

type DefaultOptions struct {
	model           *string
	maxTokens       *int64
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int64
	topP            *float64
}

func (d *DefaultOptions) Model() *string {
	return d.model
}

func (d *DefaultOptions) MaxTokens() *int64 {
	return d.maxTokens
}

func (d *DefaultOptions) PresencePenalty() *float64 {
	return d.presencePenalty
}

func (d *DefaultOptions) StopSequences() []string {
	return d.stopSequences
}

func (d *DefaultOptions) Temperature() *float64 {
	return d.temperature
}

func (d *DefaultOptions) TopK() *int64 {
	return d.topK
}

func (d *DefaultOptions) TopP() *float64 {
	return d.topP
}

func (d *DefaultOptions) Copy() Options {
	builder := NewOptionsBuilder()
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
	return builder.Build()
}

type DefaultOptionsBuilder struct {
	options *DefaultOptions
}

func NewOptionsBuilder() *DefaultOptionsBuilder {
	return &DefaultOptionsBuilder{
		options: &DefaultOptions{},
	}
}

func (d *DefaultOptionsBuilder) WithModel(model string) *DefaultOptionsBuilder {
	d.options.model = &model
	return d
}
func (d *DefaultOptionsBuilder) WithMaxTokens(maxTokens int64) *DefaultOptionsBuilder {
	d.options.maxTokens = &maxTokens
	return d
}
func (d *DefaultOptionsBuilder) WithPresencePenalty(presencePenalty float64) *DefaultOptionsBuilder {
	d.options.presencePenalty = &presencePenalty
	return d
}
func (d *DefaultOptionsBuilder) WithStopSequences(stopSequences []string) *DefaultOptionsBuilder {
	d.options.stopSequences = stopSequences
	return d
}
func (d *DefaultOptionsBuilder) WithTemperature(temperature float64) *DefaultOptionsBuilder {
	d.options.temperature = &temperature
	return d
}
func (d *DefaultOptionsBuilder) WithTopK(topK int64) *DefaultOptionsBuilder {
	d.options.topK = &topK
	return d
}
func (d *DefaultOptionsBuilder) WithTopP(topP float64) *DefaultOptionsBuilder {
	d.options.topP = &topP
	return d
}
func (d *DefaultOptionsBuilder) Build() Options {
	return d.options
}
