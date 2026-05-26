package bedrock

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Tangerg/lynx/core/media"
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
	api            *API
	defaultOptions *chat.Options
}

func NewChatModel(ctx context.Context, cfg *ChatModelConfig) (*ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	api, err := NewAPI(ctx, &APIConfig{Region: cfg.Region, AWSConfig: cfg.AWSConfig})
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
			for _, mm := range m.Media {
				if block := mediaToBlock(mm); block != nil {
					blocks = append(blocks, block)
				}
			}
			if len(blocks) > 0 {
				out = append(out, types.Message{
					Role:    types.ConversationRoleUser,
					Content: blocks,
				})
			}
		case *chat.AssistantMessage:
			blocks := []types.ContentBlock{}
			// Bedrock Converse preserves ContentBlock ordering — map
			// Parts 1:1 in emission order.
			for _, p := range m.Parts {
				switch tp := p.(type) {
				case *chat.TextPart:
					if tp.Text != "" {
						blocks = append(blocks, &types.ContentBlockMemberText{Value: tp.Text})
					}
				case *chat.ReasoningPart:
					// Round-trip the model's reasoning back to the
					// provider so multi-turn thinking is preserved.
					// Signature must be echoed verbatim — Bedrock
					// rejects the call when it's tampered with.
					if tp.Text == "" && len(tp.Signature) == 0 {
						continue
					}
					rb := types.ReasoningTextBlock{}
					if tp.Text != "" {
						rb.Text = aws.String(tp.Text)
					}
					if len(tp.Signature) > 0 {
						rb.Signature = aws.String(string(tp.Signature))
					}
					blocks = append(blocks, &types.ContentBlockMemberReasoningContent{
						Value: &types.ReasoningContentBlockMemberReasoningText{Value: rb},
					})
				case *chat.ToolCallPart:
					var input map[string]any
					if tp.Arguments != "" {
						_ = json.Unmarshal([]byte(tp.Arguments), &input)
					}
					blocks = append(blocks, &types.ContentBlockMemberToolUse{
						Value: types.ToolUseBlock{
							ToolUseId: aws.String(tp.ID),
							Name:      aws.String(tp.Name),
							Input:     toDocument(input),
						},
					})
				}
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
// shape Bedrock expects for tool inputs. nil/empty inputs return nil
// so the field can stay unset on the wire.
func toDocument(v any) document.Interface {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok && len(m) == 0 {
		return nil
	}
	return document.NewLazyDocument(v)
}

// mediaToBlock maps a [*media.Media] payload onto the appropriate
// Bedrock content-block variant. Only image media is fully supported
// today — Bedrock Converse expects png / jpeg / gif / webp inline
// bytes. Unrecognised media types are silently dropped (the assistant
// gets the text portion of the message and the caller learns of the
// gap by inspecting the prompt that round-tripped).
func mediaToBlock(m *media.Media) types.ContentBlock {
	if m == nil || m.MimeType == nil {
		return nil
	}
	if !strings.EqualFold(m.MimeType.Type(), "image") {
		return nil
	}
	format, ok := bedrockImageFormat(m.MimeType.SubType())
	if !ok {
		return nil
	}
	raw, err := m.DataAsBytes()
	if err != nil || len(raw) == 0 {
		return nil
	}
	return &types.ContentBlockMemberImage{
		Value: types.ImageBlock{
			Format: format,
			Source: &types.ImageSourceMemberBytes{Value: raw},
		},
	}
}

func bedrockImageFormat(subtype string) (types.ImageFormat, bool) {
	switch strings.ToLower(subtype) {
	case "png":
		return types.ImageFormatPng, true
	case "jpeg", "jpg":
		return types.ImageFormatJpeg, true
	case "gif":
		return types.ImageFormatGif, true
	case "webp":
		return types.ImageFormatWebp, true
	default:
		return "", false
	}
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
	// Bedrock Converse preserves block order in the response — build
	// Parts in the same order as the wire.
	for _, block := range msgOut.Value.Content {
		switch b := block.(type) {
		case *types.ContentBlockMemberText:
			msgParams.Parts = append(msgParams.Parts, &chat.TextPart{Text: b.Value})
		case *types.ContentBlockMemberReasoningContent:
			if rt, ok := b.Value.(*types.ReasoningContentBlockMemberReasoningText); ok {
				rp := &chat.ReasoningPart{}
				if rt.Value.Text != nil {
					rp.Text = *rt.Value.Text
				}
				if rt.Value.Signature != nil {
					rp.Signature = []byte(*rt.Value.Signature)
				}
				if rp.Text != "" || len(rp.Signature) > 0 {
					msgParams.Parts = append(msgParams.Parts, rp)
				}
			}
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
			msgParams.Parts = append(msgParams.Parts, &chat.ToolCallPart{
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
		meta.Usage = bedrockUsageToChat(out.Usage)
	}
	return chat.NewResponse(result, meta)
}

// bedrockUsageToChat maps the Bedrock Usage struct (which itself
// varies a bit by model — Anthropic Bedrock surfaces cache tokens,
// Llama/Titan don't) onto [chat.Usage], preserving the original
// payload via OriginalUsage so callers can drop down when needed.
func bedrockUsageToChat(u *types.TokenUsage) *chat.Usage {
	if u == nil {
		return nil
	}
	usage := &chat.Usage{OriginalUsage: u}
	if u.InputTokens != nil {
		usage.PromptTokens = int64(*u.InputTokens)
	}
	if u.OutputTokens != nil {
		usage.CompletionTokens = int64(*u.OutputTokens)
	}
	if u.CacheReadInputTokens != nil {
		v := int64(*u.CacheReadInputTokens)
		usage.CacheReadInputTokens = &v
	}
	if u.CacheWriteInputTokens != nil {
		v := int64(*u.CacheWriteInputTokens)
		usage.CacheWriteInputTokens = &v
	}
	return usage
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
// only cross-event state needed is tool-call ID routing:
//
//   - blockToToolID: maps Bedrock's content_block_index → the
//     ToolCallPart.ID assigned at start time, so subsequent
//     input-delta events can attach to the right tool call.
type chunkAccumulator struct {
	blockToToolID map[int64]string
}

// newChunkAccumulator returns an empty accumulator. Call
// [chunkAccumulator.AddChunk] for each event in order.
func newChunkAccumulator() *chunkAccumulator {
	return &chunkAccumulator{blockToToolID: map[int64]string{}}
}

// AddChunk converts one Converse stream event into a per-event delta
// [*chat.Response]. Returns (nil, false) for events that produced no
// observable content.
func (a *chunkAccumulator) AddChunk(evt types.ConverseStreamOutput) (*chat.Response, bool) {
	var msgParams chat.MessageParams
	resultMeta := &chat.ResultMetadata{}
	var metaUsage *chat.Usage
	hasContent := false

	switch e := evt.(type) {
	case *types.ConverseStreamOutputMemberContentBlockStart:
		tu, ok := e.Value.Start.(*types.ContentBlockStartMemberToolUse)
		if !ok {
			break
		}
		id, name := "", ""
		if tu.Value.ToolUseId != nil {
			id = *tu.Value.ToolUseId
		}
		if tu.Value.Name != nil {
			name = *tu.Value.Name
		}
		if e.Value.ContentBlockIndex != nil {
			a.blockToToolID[int64(*e.Value.ContentBlockIndex)] = id
		}
		msgParams.Parts = []chat.OutputPart{&chat.ToolCallPart{
			ID:   id,
			Name: name,
		}}
		hasContent = true

	case *types.ConverseStreamOutputMemberContentBlockDelta:
		switch d := e.Value.Delta.(type) {
		case *types.ContentBlockDeltaMemberText:
			if d.Value == "" {
				break
			}
			msgParams.Parts = []chat.OutputPart{&chat.TextPart{Text: d.Value}}
			hasContent = true
		case *types.ContentBlockDeltaMemberReasoningContent:
			switch rd := d.Value.(type) {
			case *types.ReasoningContentBlockDeltaMemberText:
				if rd.Value == "" {
					break
				}
				msgParams.Parts = []chat.OutputPart{&chat.ReasoningPart{Text: rd.Value}}
				hasContent = true
			case *types.ReasoningContentBlockDeltaMemberSignature:
				if rd.Value == "" {
					break
				}
				msgParams.Parts = []chat.OutputPart{&chat.ReasoningPart{
					Signature: []byte(rd.Value),
				}}
				hasContent = true
			}
		case *types.ContentBlockDeltaMemberToolUse:
			if d.Value.Input == nil || *d.Value.Input == "" {
				break
			}
			id := ""
			if e.Value.ContentBlockIndex != nil {
				id = a.blockToToolID[int64(*e.Value.ContentBlockIndex)]
			}
			msgParams.Parts = []chat.OutputPart{&chat.ToolCallPart{
				ID:        id,
				Arguments: *d.Value.Input,
			}}
			hasContent = true
		}

	case *types.ConverseStreamOutputMemberMessageStop:
		resultMeta.FinishReason = mapStopReason(e.Value.StopReason)
		hasContent = true

	case *types.ConverseStreamOutputMemberMetadata:
		if e.Value.Usage == nil {
			break
		}
		metaUsage = bedrockUsageToChat(e.Value.Usage)
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
