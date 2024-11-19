package chat

import (
	"errors"
	"github.com/Tangerg/lynx/ai/core/chat/request"
	"github.com/Tangerg/lynx/ai/core/model/function"
	"github.com/samber/lo"
)

type OpenAIChatRequestOptions struct {
	model           *string
	maxTokens       *int
	presencePenalty *float64
	stopSequences   []string
	temperature     *float64
	topK            *int
	topP            *float64
	n               int
	functions       []function.Function
	proxyToolCalls  bool
}

func (o *OpenAIChatRequestOptions) Functions() []function.Function {
	return o.functions
}

func (o *OpenAIChatRequestOptions) SetFunctions(funcs []function.Function) {
	funcs = lo.Uniq(funcs)
	o.functions = funcs
}

func (o *OpenAIChatRequestOptions) ProxyToolCalls() bool {
	return o.proxyToolCalls
}

func (o *OpenAIChatRequestOptions) SetProxyToolCalls(enable bool) {
	o.proxyToolCalls = enable
}

func (o *OpenAIChatRequestOptions) Model() *string {
	return o.model
}

func (o *OpenAIChatRequestOptions) MaxTokens() *int {
	return o.maxTokens
}

func (o *OpenAIChatRequestOptions) PresencePenalty() *float64 {
	return o.presencePenalty
}

func (o *OpenAIChatRequestOptions) StopSequences() []string {
	return o.stopSequences
}

func (o *OpenAIChatRequestOptions) Temperature() *float64 {
	return o.temperature
}

func (o *OpenAIChatRequestOptions) TopK() *int {
	return o.topK
}

func (o *OpenAIChatRequestOptions) TopP() *float64 {
	return o.topP
}

func (o *OpenAIChatRequestOptions) N() int {
	if o.n < 1 {
		o.n = 1
	}
	return o.n
}

func (o *OpenAIChatRequestOptions) Clone() request.ChatRequestOptions {
	builder := NewOpenAIChatRequestOptionsBuilder()
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
	if o.functions != nil {
		builder.WithFunctions(o.functions...)
	}
	builder.WithProxyToolCalls(o.proxyToolCalls)
	builder.WithN(o.n)

	cp, _ := builder.Build()
	return cp
}

type OpenAIChatRequestOptionsBuilder struct {
	options *OpenAIChatRequestOptions
}

func NewOpenAIChatRequestOptionsBuilder() *OpenAIChatRequestOptionsBuilder {
	return &OpenAIChatRequestOptionsBuilder{
		options: &OpenAIChatRequestOptions{},
	}
}

func (o *OpenAIChatRequestOptionsBuilder) WithModel(model string) *OpenAIChatRequestOptionsBuilder {
	o.options.model = &model
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithMaxTokens(maxTokens int) *OpenAIChatRequestOptionsBuilder {
	o.options.maxTokens = &maxTokens
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithPresencePenalty(presencePenalty float64) *OpenAIChatRequestOptionsBuilder {
	o.options.presencePenalty = &presencePenalty
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithStopSequences(stopSequences []string) *OpenAIChatRequestOptionsBuilder {
	o.options.stopSequences = stopSequences
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithTemperature(temperature float64) *OpenAIChatRequestOptionsBuilder {
	o.options.temperature = &temperature
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithTopK(topK int) *OpenAIChatRequestOptionsBuilder {
	o.options.topK = &topK
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithTopP(topP float64) *OpenAIChatRequestOptionsBuilder {
	o.options.topP = &topP
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithN(n int) *OpenAIChatRequestOptionsBuilder {
	o.options.n = n
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithFunctions(funcs ...function.Function) *OpenAIChatRequestOptionsBuilder {
	o.options.SetFunctions(funcs)
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) WithProxyToolCalls(enable bool) *OpenAIChatRequestOptionsBuilder {
	o.options.proxyToolCalls = enable
	return o
}
func (o *OpenAIChatRequestOptionsBuilder) Build() (*OpenAIChatRequestOptions, error) {
	if o.options.model == nil {
		return nil, errors.New("model is required")
	}
	return o.options, nil
}
