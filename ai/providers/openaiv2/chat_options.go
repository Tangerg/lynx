package openaiv2

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/assert"
)

var _ chat.Options = (*ChatOptions)(nil)
var _ tool.Options = (*ChatOptions)(nil)

type ChatOptions struct {
	model            string
	frequencyPenalty *float64
	maxTokens        *int64
	presencePenalty  *float64
	stopSequences    []string
	temperature      *float64
	topK             *int
	topP             *float64
	tools            []tool.Tool
	toolParams       map[string]any
}

func (o *ChatOptions) Tools() []tool.Tool {
	return o.tools
}

func (o *ChatOptions) SetTools(tools []tool.Tool) {
	for _, t := range tools {
		o.tools = append(o.tools, t)
	}
}

func (o *ChatOptions) ToolParams() map[string]any {
	if o.toolParams == nil {
		o.toolParams = make(map[string]any)
	}
	return o.toolParams
}

func (o *ChatOptions) SetToolParams(params map[string]any) {
	if o.toolParams == nil {
		o.toolParams = make(map[string]any)
	}
	if len(params) == 0 {
		return
	}
	for k, v := range params {
		o.toolParams[k] = v
	}
}

func (o *ChatOptions) Model() string {
	return o.model
}

func (o *ChatOptions) FrequencyPenalty() *float64 {
	return o.frequencyPenalty
}

func (o *ChatOptions) MaxTokens() *int64 {
	return o.maxTokens
}

func (o *ChatOptions) PresencePenalty() *float64 {
	return o.presencePenalty
}

func (o *ChatOptions) StopSequences() []string {
	return o.stopSequences
}

func (o *ChatOptions) Temperature() *float64 {
	return o.temperature
}

func (o *ChatOptions) TopK() *int {
	return o.topK
}

func (o *ChatOptions) TopP() *float64 {
	return o.topP
}

func (o *ChatOptions) Clone() chat.Options {
	return o.Fork().
		MustBuild()
}

func (o *ChatOptions) Fork() *ChatOptionsBuilder {
	return NewChatOptionsBuilder().
		WithModel(o.model).
		WithFrequencyPenalty(o.frequencyPenalty).
		WithMaxTokens(o.maxTokens).
		WithPresencePenalty(o.presencePenalty).
		WithStopSequences(o.stopSequences).
		WithTemperature(o.temperature).
		WithTopK(o.topK).
		WithTopP(o.topP).
		WithTools(o.tools).
		WithToolParams(o.toolParams)
}

type ChatOptionsBuilder struct {
	model            string
	frequencyPenalty *float64
	maxTokens        *int64
	presencePenalty  *float64
	stopSequences    []string
	temperature      *float64
	topK             *int
	topP             *float64
	tools            []tool.Tool
	toolParams       map[string]any
}

func NewChatOptionsBuilder() *ChatOptionsBuilder {
	return &ChatOptionsBuilder{}
}

func (b *ChatOptionsBuilder) WithModel(model string) *ChatOptionsBuilder {
	b.model = model
	return b
}

func (b *ChatOptionsBuilder) WithFrequencyPenalty(penalty *float64) *ChatOptionsBuilder {
	if penalty != nil {
		b.frequencyPenalty = penalty
	}
	return b
}

func (b *ChatOptionsBuilder) WithMaxTokens(maxTokens *int64) *ChatOptionsBuilder {
	if maxTokens != nil {
		b.maxTokens = maxTokens
	}
	return b
}

func (b *ChatOptionsBuilder) WithPresencePenalty(penalty *float64) *ChatOptionsBuilder {
	if penalty != nil {
		b.presencePenalty = penalty
	}
	return b
}

func (b *ChatOptionsBuilder) WithStopSequences(sequences []string) *ChatOptionsBuilder {
	b.stopSequences = sequences
	return b
}

func (b *ChatOptionsBuilder) WithTemperature(temperature *float64) *ChatOptionsBuilder {
	if temperature != nil {
		b.temperature = temperature
	}
	return b
}

func (b *ChatOptionsBuilder) WithTopK(topK *int) *ChatOptionsBuilder {
	if topK != nil {
		b.topK = topK
	}
	return b
}

func (b *ChatOptionsBuilder) WithTopP(topP *float64) *ChatOptionsBuilder {
	if topP != nil {
		b.topP = topP
	}
	return b
}

func (b *ChatOptionsBuilder) WithTools(tools []tool.Tool) *ChatOptionsBuilder {
	if len(tools) > 0 {
		b.tools = tools
	}
	return b
}

func (b *ChatOptionsBuilder) WithToolParams(params map[string]any) *ChatOptionsBuilder {
	if len(params) > 0 {
		b.toolParams = params
	}
	return b
}

func (b *ChatOptionsBuilder) Build() (*ChatOptions, error) {
	if b.model == "" {
		return nil, errors.New("model is required")
	}
	return &ChatOptions{
		model:            b.model,
		frequencyPenalty: b.frequencyPenalty,
		maxTokens:        b.maxTokens,
		presencePenalty:  b.presencePenalty,
		stopSequences:    b.stopSequences,
		temperature:      b.temperature,
		topK:             b.topK,
		topP:             b.topP,
	}, nil
}

func (b *ChatOptionsBuilder) MustBuild() *ChatOptions {
	return assert.ErrorIsNil(b.Build())
}
