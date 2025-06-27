package openai

import (
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/assert"
)

var _ tool.Options = (*ChatOptions)(nil)

type ChatOptions struct {
	model               string
	frequencyPenalty    *float64
	logitBias           map[string]int64
	logprobs            *bool
	maxCompletionTokens *int64
	maxTokens           *int64
	metadata            map[string]string
	modalities          []string
	n                   *int64
	parallelToolCalls   *bool
	presencePenalty     *float64
	reasoningEffort     *string
	seed                *int64
	serviceTier         *string
	stop                []string
	store               *bool
	temperature         *float64
	topLogprobs         *int64
	topP                *float64
	user                *string
	tools               []tool.Tool
	toolParams          map[string]any
}

func (o *ChatOptions) Tools() []tool.Tool {
	return o.tools
}

func (o *ChatOptions) SetTools(tools []tool.Tool) {
	if tools != nil {
		o.tools = tools
	}
}

func (o *ChatOptions) AddTools(tools []tool.Tool) {
	for _, t := range tools {
		o.tools = append(o.tools, t)
	}
}

func (o *ChatOptions) ensureToolParams() {
	if o.toolParams == nil {
		o.toolParams = make(map[string]any)
	}
}

func (o *ChatOptions) ToolParams() map[string]any {
	o.ensureToolParams()
	return o.toolParams
}

func (o *ChatOptions) SetToolParams(params map[string]any) {
	o.ensureToolParams()

	if params != nil {
		o.toolParams = params
	}
}

func (o *ChatOptions) AddToolParams(params map[string]any) {
	o.ensureToolParams()

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

func (o *ChatOptions) LogitBias() map[string]int64 {
	return o.logitBias
}

func (o *ChatOptions) Logprobs() *bool {
	return o.logprobs
}

func (o *ChatOptions) MaxCompletionTokens() *int64 {
	return o.maxCompletionTokens
}

func (o *ChatOptions) MaxTokens() *int64 {
	return o.maxTokens
}

func (o *ChatOptions) Metadata() map[string]string {
	return o.metadata
}

func (o *ChatOptions) Modalities() []string {
	return o.modalities
}

func (o *ChatOptions) N() *int64 {
	return o.n
}

func (o *ChatOptions) ParallelToolCalls() *bool {
	return o.parallelToolCalls
}

func (o *ChatOptions) PresencePenalty() *float64 {
	return o.presencePenalty
}

func (o *ChatOptions) ReasoningEffort() *string {
	return o.reasoningEffort
}

func (o *ChatOptions) Seed() *int64 {
	return o.seed
}

func (o *ChatOptions) ServiceTier() *string {
	return o.serviceTier
}

func (o *ChatOptions) Stop() []string {
	return o.stop
}

func (o *ChatOptions) Store() *bool {
	return o.store
}

func (o *ChatOptions) Temperature() *float64 {
	return o.temperature
}

func (o *ChatOptions) TopLogprobs() *int64 {
	return o.topLogprobs
}

func (o *ChatOptions) TopK() *int64 {
	return nil
}

func (o *ChatOptions) TopP() *float64 {
	return o.topP
}

func (o *ChatOptions) User() *string {
	return o.user
}

func (o *ChatOptions) Clone() chat.Options {
	return o.Fork().MustBuild()
}

func (o *ChatOptions) Fork() *ChatOptionsBuilder {
	builder := NewChatOptionsBuilder().
		WithModel(o.model).
		WithFrequencyPenalty(o.frequencyPenalty).
		WithMaxTokens(o.maxTokens).
		WithPresencePenalty(o.presencePenalty).
		WithStop(o.stop).
		WithTemperature(o.temperature).
		WithTopP(o.topP).
		WithTools(o.tools).
		WithToolParams(o.toolParams).
		WithLogitBias(o.logitBias).
		WithLogprobs(o.logprobs).
		WithMaxCompletionTokens(o.maxCompletionTokens).
		WithMetadata(o.metadata).
		WithModalities(o.modalities).
		WithN(o.n).
		WithParallelToolCalls(o.parallelToolCalls).
		WithReasoningEffort(o.reasoningEffort).
		WithSeed(o.seed).
		WithServiceTier(o.serviceTier).
		WithStore(o.store).
		WithTopLogprobs(o.topLogprobs).
		WithUser(o.user)

	return builder
}

type ChatOptionsBuilder struct {
	model               string
	frequencyPenalty    *float64
	logitBias           map[string]int64
	logprobs            *bool
	maxCompletionTokens *int64
	maxTokens           *int64
	metadata            map[string]string
	modalities          []string
	n                   *int64
	parallelToolCalls   *bool
	presencePenalty     *float64
	reasoningEffort     *string
	seed                *int64
	serviceTier         *string
	stop                []string
	store               *bool
	temperature         *float64
	topLogprobs         *int64
	topP                *float64
	user                *string
	tools               []tool.Tool
	toolParams          map[string]any
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

func (b *ChatOptionsBuilder) WithLogitBias(logitBias map[string]int64) *ChatOptionsBuilder {
	if len(logitBias) > 0 {
		b.logitBias = logitBias
	}
	return b
}

func (b *ChatOptionsBuilder) WithLogprobs(logprobs *bool) *ChatOptionsBuilder {
	if logprobs != nil {
		b.logprobs = logprobs
	}
	return b
}

func (b *ChatOptionsBuilder) WithMaxCompletionTokens(maxTokens *int64) *ChatOptionsBuilder {
	if maxTokens != nil {
		b.maxCompletionTokens = maxTokens
	}
	return b
}

func (b *ChatOptionsBuilder) WithMaxTokens(maxTokens *int64) *ChatOptionsBuilder {
	if maxTokens != nil {
		b.maxTokens = maxTokens
	}
	return b
}

func (b *ChatOptionsBuilder) WithMetadata(metadata map[string]string) *ChatOptionsBuilder {
	if len(metadata) > 0 {
		b.metadata = metadata
	}
	return b
}

func (b *ChatOptionsBuilder) WithModalities(modalities []string) *ChatOptionsBuilder {
	if len(modalities) > 0 {
		b.modalities = modalities
	}
	return b
}

func (b *ChatOptionsBuilder) WithN(n *int64) *ChatOptionsBuilder {
	if n != nil {
		b.n = n
	}
	return b
}

func (b *ChatOptionsBuilder) WithParallelToolCalls(parallel *bool) *ChatOptionsBuilder {
	if parallel != nil {
		b.parallelToolCalls = parallel
	}
	return b
}

func (b *ChatOptionsBuilder) WithPresencePenalty(penalty *float64) *ChatOptionsBuilder {
	if penalty != nil {
		b.presencePenalty = penalty
	}
	return b
}

func (b *ChatOptionsBuilder) WithReasoningEffort(effort *string) *ChatOptionsBuilder {
	if effort != nil {
		b.reasoningEffort = effort
	}
	return b
}

func (b *ChatOptionsBuilder) WithSeed(seed *int64) *ChatOptionsBuilder {
	if seed != nil {
		b.seed = seed
	}
	return b
}

func (b *ChatOptionsBuilder) WithServiceTier(tier *string) *ChatOptionsBuilder {
	if tier != nil {
		b.serviceTier = tier
	}
	return b
}

func (b *ChatOptionsBuilder) WithStop(stop []string) *ChatOptionsBuilder {
	if len(stop) > 0 {
		b.stop = stop
	}
	return b
}

func (b *ChatOptionsBuilder) WithStore(store *bool) *ChatOptionsBuilder {
	if store != nil {
		b.store = store
	}
	return b
}

func (b *ChatOptionsBuilder) WithTemperature(temperature *float64) *ChatOptionsBuilder {
	if temperature != nil {
		b.temperature = temperature
	}
	return b
}

func (b *ChatOptionsBuilder) WithTopLogprobs(topLogprobs *int64) *ChatOptionsBuilder {
	if topLogprobs != nil {
		b.topLogprobs = topLogprobs
	}
	return b
}

func (b *ChatOptionsBuilder) WithTopP(topP *float64) *ChatOptionsBuilder {
	if topP != nil {
		b.topP = topP
	}
	return b
}

func (b *ChatOptionsBuilder) WithUser(user *string) *ChatOptionsBuilder {
	if user != nil {
		b.user = user
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
		model:               b.model,
		frequencyPenalty:    b.frequencyPenalty,
		logitBias:           b.logitBias,
		logprobs:            b.logprobs,
		maxCompletionTokens: b.maxCompletionTokens,
		maxTokens:           b.maxTokens,
		metadata:            b.metadata,
		modalities:          b.modalities,
		n:                   b.n,
		parallelToolCalls:   b.parallelToolCalls,
		presencePenalty:     b.presencePenalty,
		reasoningEffort:     b.reasoningEffort,
		seed:                b.seed,
		serviceTier:         b.serviceTier,
		stop:                b.stop,
		store:               b.store,
		temperature:         b.temperature,
		topLogprobs:         b.topLogprobs,
		topP:                b.topP,
		user:                b.user,
		tools:               b.tools,
		toolParams:          b.toolParams,
	}, nil
}

func (b *ChatOptionsBuilder) MustBuild() *ChatOptions {
	return assert.ErrorIsNil(b.Build())
}

func MergeChatOptions(options *ChatOptions, opts ...chat.Options) (*ChatOptions, error) {
	fork := options.Fork()
	for _, o := range opts {
		if o == nil {
			continue
		}
		fork.
			WithModel(o.Model()).
			WithFrequencyPenalty(o.FrequencyPenalty()).
			WithMaxTokens(o.MaxTokens()).
			WithPresencePenalty(o.PresencePenalty()).
			WithStop(o.Stop()).
			WithTemperature(o.Temperature()).
			WithTopP(o.TopP())

		toolOptions, ok := o.(tool.Options)
		if ok {
			fork.
				WithTools(toolOptions.Tools()).
				WithToolParams(toolOptions.ToolParams())
		}

		chatOptions, ok := o.(*ChatOptions)
		if ok {
			fork.
				WithLogitBias(chatOptions.logitBias).
				WithLogprobs(chatOptions.logprobs).
				WithMaxCompletionTokens(chatOptions.maxCompletionTokens).
				WithMetadata(chatOptions.metadata).
				WithModalities(chatOptions.modalities).
				WithN(chatOptions.n).
				WithParallelToolCalls(chatOptions.parallelToolCalls).
				WithReasoningEffort(chatOptions.reasoningEffort).
				WithSeed(chatOptions.seed).
				WithServiceTier(chatOptions.serviceTier).
				WithStore(chatOptions.store).
				WithTopLogprobs(chatOptions.topLogprobs).
				WithUser(chatOptions.user)
		}
	}
	return fork.Build()
}
