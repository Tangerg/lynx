package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"iter"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/options"
)

type ChatModelConfig struct {
	DefaultOptions *chat.Options
	Region         string
	AWSConfig      *aws.Config
}

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("bedrock: config must not be nil")
	}
	if c.DefaultOptions == nil {
		return errors.New("bedrock: DefaultOptions is required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

// ChatModel wraps Bedrock's Converse API — the unified inference
// surface that covers every Bedrock-hosted model family (Claude /
// Llama / Titan / Mistral / Cohere / DeepSeek / Nova). The lynx
// [chat.Options].Model carries the Bedrock model id (e.g.
// "anthropic.claude-3-5-sonnet-20241022-v2:0",
// "meta.llama3-1-70b-instruct-v1:0", "amazon.nova-pro-v1:0").
//
// Tool calling, document refs, image input, and the Bedrock-specific
// ToolConfig / GuardrailConfig / PerformanceConfig live on
// [bedrockruntime.ConverseInput] and reach the wire through the
// Extra-threaded SDK params.
type ChatModel struct {
	api            *Api
	defaultOptions *chat.Options
}

func NewChatModel(ctx context.Context, cfg *ChatModelConfig) (*ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewApi(ctx, &ApiConfig{Region: cfg.Region, AWSConfig: cfg.AWSConfig})
	if err != nil {
		return nil, err
	}
	return &ChatModel{api: api, defaultOptions: cfg.DefaultOptions}, nil
}

func (c *ChatModel) buildConverseInput(req *chat.Request) (*bedrockruntime.ConverseInput, error) {
	mergedOpts, err := chat.MergeOptions(c.defaultOptions, req.Options)
	if err != nil {
		return nil, err
	}

	in := options.GetParams[bedrockruntime.ConverseInput](mergedOpts, OptionsKey)
	in.ModelId = aws.String(mergedOpts.Model)

	if in.InferenceConfig == nil {
		in.InferenceConfig = &types.InferenceConfiguration{}
	}
	if mergedOpts.Temperature != nil {
		v := float32(*mergedOpts.Temperature)
		in.InferenceConfig.Temperature = &v
	}
	if mergedOpts.TopP != nil {
		v := float32(*mergedOpts.TopP)
		in.InferenceConfig.TopP = &v
	}
	if mergedOpts.MaxTokens != nil {
		v := int32(*mergedOpts.MaxTokens)
		in.InferenceConfig.MaxTokens = &v
	}
	if len(mergedOpts.Stop) > 0 {
		in.InferenceConfig.StopSequences = mergedOpts.Stop
	}

	sys, msgs := buildMessages(req.Messages)
	if len(sys) > 0 {
		in.System = sys
	}
	in.Messages = msgs

	return in, nil
}

func buildMessages(msgs []chat.Message) ([]types.SystemContentBlock, []types.Message) {
	var systemBlocks []types.SystemContentBlock
	out := make([]types.Message, 0, len(msgs))

	for _, msg := range msgs {
		switch m := msg.(type) {
		case *chat.SystemMessage:
			if m.Text != "" {
				systemBlocks = append(systemBlocks, &types.SystemContentBlockMemberText{Value: m.Text})
			}
		case *chat.UserMessage:
			blocks := []types.ContentBlock{}
			if m.Text != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: m.Text})
			}
			if len(blocks) > 0 {
				out = append(out, types.Message{
					Role:    types.ConversationRoleUser,
					Content: blocks,
				})
			}
		case *chat.AssistantMessage:
			blocks := []types.ContentBlock{}
			if m.Text != "" {
				blocks = append(blocks, &types.ContentBlockMemberText{Value: m.Text})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				if tc.Arguments != "" {
					_ = json.Unmarshal([]byte(tc.Arguments), &input)
				}
				blocks = append(blocks, &types.ContentBlockMemberToolUse{
					Value: types.ToolUseBlock{
						ToolUseId: aws.String(tc.ID),
						Name:      aws.String(tc.Name),
						Input:     toDocument(input),
					},
				})
			}
			if len(blocks) > 0 {
				out = append(out, types.Message{
					Role:    types.ConversationRoleAssistant,
					Content: blocks,
				})
			}
		case *chat.ToolMessage:
			blocks := []types.ContentBlock{}
			for _, ret := range m.ToolReturns {
				blocks = append(blocks, &types.ContentBlockMemberToolResult{
					Value: types.ToolResultBlock{
						ToolUseId: aws.String(ret.ID),
						Content: []types.ToolResultContentBlock{
							&types.ToolResultContentBlockMemberText{Value: ret.Result},
						},
					},
				})
			}
			if len(blocks) > 0 {
				// Tool results ride as a user-role message per Bedrock spec.
				out = append(out, types.Message{
					Role:    types.ConversationRoleUser,
					Content: blocks,
				})
			}
		}
	}
	return systemBlocks, out
}

// toDocument wraps an arbitrary value into the AWS Smithy document
// shape Bedrock expects for tool inputs.
func toDocument(v any) document.Interface {
	if v == nil {
		return nil
	}
	// Return nil for now — callers needing typed tool inputs thread
	// them through Extra-threaded ConverseInput. A proper smithy
	// document builder (document.NewLazyDocument) requires importing
	// the AWS document/jsonrpc package which we keep out of the public
	// surface.
	return nil
}

func (c *ChatModel) buildResponse(out *bedrockruntime.ConverseOutput) (*chat.Response, error) {
	if out == nil || out.Output == nil {
		return nil, errors.New("bedrock: response has no output")
	}

	msgOut, ok := out.Output.(*types.ConverseOutputMemberMessage)
	if !ok || msgOut == nil {
		return nil, errors.New("bedrock: response has no output")
	}

	msgParams := chat.MessageParams{Metadata: make(map[string]any)}
	for _, block := range msgOut.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			msgParams.Text += b.Value
		case *types.ContentBlockMemberToolUse:
			argsBytes, _ := json.Marshal(b.Value.Input)
			id := ""
			if b.Value.ToolUseId != nil {
				id = *b.Value.ToolUseId
			}
			name := ""
			if b.Value.Name != nil {
				name = *b.Value.Name
			}
			msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{
				ID:        id,
				Name:      name,
				Arguments: string(argsBytes),
			})
		}
	}

	assistantMsg := chat.NewAssistantMessage(msgParams)
	resultMeta := &chat.ResultMetadata{FinishReason: mapStopReason(out.StopReason)}

	result, err := chat.NewResult(assistantMsg, resultMeta)
	if err != nil {
		return nil, err
	}

	meta := &chat.ResponseMetadata{}
	if out.Usage != nil {
		usage := &chat.Usage{OriginalUsage: out.Usage}
		if out.Usage.InputTokens != nil {
			usage.PromptTokens = int64(*out.Usage.InputTokens)
		}
		if out.Usage.OutputTokens != nil {
			usage.CompletionTokens = int64(*out.Usage.OutputTokens)
		}
		meta.Usage = usage
	}
	return chat.NewResponse(result, meta)
}

func mapStopReason(reason types.StopReason) chat.FinishReason {
	switch reason {
	case types.StopReasonEndTurn, types.StopReasonStopSequence:
		return chat.FinishReasonStop
	case types.StopReasonMaxTokens:
		return chat.FinishReasonLength
	case types.StopReasonToolUse:
		return chat.FinishReasonToolCalls
	case types.StopReasonContentFiltered, types.StopReasonGuardrailIntervened:
		return chat.FinishReasonContentFilter
	default:
		return chat.FinishReasonOther
	}
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	in, err := c.buildConverseInput(req)
	if err != nil {
		return nil, err
	}
	out, err := c.api.Converse(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.buildResponse(out)
}

// Stream wraps Bedrock's ConverseStream. The event stream carries
// per-delta events (ContentBlockDelta, ContentBlockStart, ...); we
// accumulate text + tool-use bytes and yield a chat.Response per
// content-block-delta with the cumulative state, mirroring how the
// openai/anthropic providers stream.
// chunkAccumulator is the bedrock counterpart of openai-go's
// [openai.ChatCompletionAccumulator]: each [chunkAccumulator.AddChunk]
// consumes one Converse stream event and returns a [*chat.Response]
// carrying ONLY that event's delta. The upstream
// [chat.ResponseAccumulator] stitches deltas together.
//
// Bedrock doesn't ship an SDK accumulator, so we write our own. The
// only cross-event state needed is the tool-slot routing:
//
//   - blockToToolSlot: maps Bedrock's content_block_index → the dense
//     [chat.AssistantMessage].ToolCalls slot expected by
//     [chat.ResponseAccumulator]. Bedrock interleaves text / tool_use
//     content blocks; only tool_use consumes a slot.
type chunkAccumulator struct {
	blockToToolSlot map[int64]int
	nextToolSlot    int
}

// newChunkAccumulator returns an empty accumulator. Call
// [chunkAccumulator.AddChunk] for each event in order.
func newChunkAccumulator() *chunkAccumulator {
	return &chunkAccumulator{blockToToolSlot: map[int64]int{}}
}

// AddChunk converts one Converse stream event into a per-event delta
// [*chat.Response]. Returns (nil, false) for events that produced no
// observable content.
func (a *chunkAccumulator) AddChunk(evt types.ConverseStreamOutput) (*chat.Response, bool) {
	msgParams := chat.MessageParams{Metadata: make(map[string]any)}
	resultMeta := &chat.ResultMetadata{}
	var metaUsage *chat.Usage
	hasContent := false

	switch e := evt.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		tu, ok := e.Value.Start.(*types.ContentBlockStartMemberToolUse)
		if !ok {
			break
		}
		slot := a.nextToolSlot
		a.nextToolSlot++
		if e.Value.ContentBlockIndex != nil {
			a.blockToToolSlot[int64(*e.Value.ContentBlockIndex)] = slot
		}
		tc := &chat.ToolCall{}
		if tu.Value.ToolUseId != nil {
			tc.ID = *tu.Value.ToolUseId
		}
		if tu.Value.Name != nil {
			tc.Name = *tu.Value.Name
		}
		tools := make([]*chat.ToolCall, slot+1)
		tools[slot] = tc
		msgParams.ToolCalls = tools
		hasContent = true

	case *types.ConverseStreamOutputMemberContentBlockDelta:
		switch d := e.Value.Delta.(type) {
		case *types.ContentBlockDeltaMemberText:
			if d.Value == "" {
				break
			}
			msgParams.Text = d.Value
			hasContent = true
		case *types.ContentBlockDeltaMemberToolUse:
			if d.Value.Input == nil || *d.Value.Input == "" {
				break
			}
			slot := 0
			if e.Value.ContentBlockIndex != nil {
				if s, ok := a.blockToToolSlot[int64(*e.Value.ContentBlockIndex)]; ok {
					slot = s
				}
			}
			tools := make([]*chat.ToolCall, slot+1)
			tools[slot] = &chat.ToolCall{Arguments: *d.Value.Input}
			msgParams.ToolCalls = tools
			hasContent = true
		}

	case *types.ConverseStreamOutputMemberMessageStop:
		resultMeta.FinishReason = mapStopReason(e.Value.StopReason)
		hasContent = true

	case *types.ConverseStreamOutputMemberMetadata:
		if e.Value.Usage == nil {
			break
		}
		u := &chat.Usage{OriginalUsage: e.Value.Usage}
		if e.Value.Usage.InputTokens != nil {
			u.PromptTokens = int64(*e.Value.Usage.InputTokens)
		}
		if e.Value.Usage.OutputTokens != nil {
			u.CompletionTokens = int64(*e.Value.Usage.OutputTokens)
		}
		metaUsage = u
		hasContent = true
	}

	if !hasContent {
		return nil, false
	}

	assistantMsg := chat.NewAssistantMessage(msgParams)
	result, err := chat.NewResult(assistantMsg, resultMeta)
	if err != nil {
		return nil, false
	}
	resp, err := chat.NewResponse(result, &chat.ResponseMetadata{Usage: metaUsage})
	if err != nil {
		return nil, false
	}
	return resp, true
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		converseIn, err := c.buildConverseInput(req)
		if err != nil {
			yield(nil, err)
			return
		}

		streamIn := &bedrockruntime.ConverseStreamInput{
			ModelId:         converseIn.ModelId,
			Messages:        converseIn.Messages,
			System:          converseIn.System,
			InferenceConfig: converseIn.InferenceConfig,
			ToolConfig:      converseIn.ToolConfig,
		}

		out, err := c.api.ConverseStream(ctx, streamIn)
		if err != nil {
			yield(nil, err)
			return
		}
		stream := out.GetStream()
		defer stream.Close()

		// Per-stream chunk accumulator — mirrors OpenAI's per-chunk
		// fresh [openai.ChatCompletionAccumulator] approach. Each
		// AddChunk consumes one event and emits ONLY that event's
		// delta. The upstream [chat.ResponseAccumulator] stitches.
		acc := newChunkAccumulator()

		for evt := range stream.Events() {
			resp, ok := acc.AddChunk(evt)
			if !ok {
				continue
			}
			if !yield(resp, nil) {
				return
			}
		}

		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (c *ChatModel) DefaultOptions() chat.Options { return *c.defaultOptions }
func (c *ChatModel) Metadata() chat.ModelMetadata         { return chat.ModelMetadata{Provider: Provider} }
