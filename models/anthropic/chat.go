package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/pkg/mime"
)

type requestHelper struct {
	defaultOptions *chat.Options
}

func (r *requestHelper) buildToolParams(tools []chat.Tool) ([]anthropicsdk.ToolUnionParam, error) {
	toolParams := make([]anthropicsdk.ToolUnionParam, 0, len(tools))

	for _, t := range tools {
		def := t.Definition()

		var schema map[string]any
		if err := json.Unmarshal([]byte(def.InputSchema), &schema); err != nil {
			return nil, err
		}
		delete(schema, "type") // auto-set to "object" by ToolInputSchemaParam

		toolParam := anthropicsdk.ToolParam{
			Name:        def.Name,
			Description: param.NewOpt(def.Description),
			InputSchema: anthropicsdk.ToolInputSchemaParam{
				ExtraFields: schema,
			},
		}

		toolParams = append(toolParams, anthropicsdk.ToolUnionParam{OfTool: &toolParam})
	}

	return toolParams, nil
}

func (r *requestHelper) buildSystem(msgs []chat.Message) []anthropicsdk.TextBlockParam {
	systemMsg := chat.MergeSystemMessages(msgs)
	if systemMsg == nil || systemMsg.Text == "" {
		return nil
	}
	return []anthropicsdk.TextBlockParam{{Text: systemMsg.Text}}
}

func (r *requestHelper) buildBaseParams(opts *chat.Options) *anthropicsdk.MessageNewParams {
	params := getOptionsParams[anthropicsdk.MessageNewParams](opts)

	params.Model = opts.Model

	if opts.MaxTokens != nil {
		params.MaxTokens = *opts.MaxTokens
	} else {
		// Anthropic requires max_tokens; default to a reasonable value
		params.MaxTokens = 4096
	}

	if opts.Temperature != nil {
		params.Temperature = param.NewOpt(*opts.Temperature)
	}
	if opts.TopP != nil {
		params.TopP = param.NewOpt(*opts.TopP)
	}
	if opts.TopK != nil {
		params.TopK = param.NewOpt(*opts.TopK)
	}
	if len(opts.Stop) > 0 {
		params.StopSequences = opts.Stop
	}

	return params
}

func (r *requestHelper) buildParams(opts *chat.Options) (*anthropicsdk.MessageNewParams, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, opts)
	if err != nil {
		return nil, err
	}

	params := r.buildBaseParams(mergedOpts)

	params.Tools, err = r.buildToolParams(mergedOpts.Tools)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func (r *requestHelper) buildUserMsg(msg *chat.UserMessage) anthropicsdk.MessageParam {
	if !msg.HasMedia() {
		return anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock(msg.Text))
	}

	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, 1+len(msg.Media))
	if msg.Text != "" {
		blocks = append(blocks, anthropicsdk.NewTextBlock(msg.Text))
	}

	for _, md := range msg.Media {
		data, err := md.DataAsString()
		if err != nil {
			continue
		}

		mt := md.MimeType
		if mime.IsImage(mt) {
			blocks = append(blocks, anthropicsdk.NewImageBlockBase64(mt.TypeAndSubType(), data))
		} else if mt.FullType() == "application/pdf" {
			blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.Base64PDFSourceParam{
				Data: data,
			}))
		} else {
			// treat other types as plain text documents
			blocks = append(blocks, anthropicsdk.NewDocumentBlock(anthropicsdk.PlainTextSourceParam{
				Data: data,
			}))
		}
	}

	return anthropicsdk.NewUserMessage(blocks...)
}

func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) anthropicsdk.MessageParam {
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, 1+len(msg.ToolCalls))

	// Replay thinking blocks first when present. Anthropic requires the
	// original signature to be echoed so the API can validate that prior
	// reasoning has not been tampered with; redacted thinking must also be
	// passed through opaquely. The signature/data live in Metadata; the
	// blocks themselves get the visible text (or empty for redacted).
	if chat.IsThoughtMessage(msg) {
		if data := chat.RedactedThinkingData(msg); data != "" {
			blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(data))
		} else if sig := chat.ThinkingSignature(msg); sig != "" {
			blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, msg.Text))
		}
	} else if msg.Text != "" {
		blocks = append(blocks, anthropicsdk.NewTextBlock(msg.Text))
	}

	for _, tc := range msg.ToolCalls {
		blocks = append(blocks, anthropicsdk.NewToolUseBlock(tc.ID, json.RawMessage(tc.Arguments), tc.Name))
	}

	return anthropicsdk.NewAssistantMessage(blocks...)
}

func (r *requestHelper) buildToolMsg(msg *chat.ToolMessage) []anthropicsdk.MessageParam {
	// Anthropic expects all tool results in a single user message
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, len(msg.ToolReturns))

	for _, ret := range msg.ToolReturns {
		blocks = append(blocks, anthropicsdk.NewToolResultBlock(ret.ID, ret.Result, false))
	}

	if len(blocks) == 0 {
		return nil
	}
	return []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(blocks...)}
}

func (r *requestHelper) buildMsg(msg chat.Message) anthropicsdk.MessageParam {
	if msg.Type().IsUser() {
		return r.buildUserMsg(msg.(*chat.UserMessage))
	}
	return r.buildAssistantMsg(msg.(*chat.AssistantMessage))
}

func (r *requestHelper) buildMsgs(msgs []chat.Message) []anthropicsdk.MessageParam {
	// Filter out system messages (handled separately)
	nonSystem := chat.FilterMessagesByMessageTypes(msgs, chat.MessageTypeUser, chat.MessageTypeAssistant, chat.MessageTypeTool)

	result := make([]anthropicsdk.MessageParam, 0, len(nonSystem))
	for _, msg := range nonSystem {
		if msg.Type().IsTool() {
			result = append(result, r.buildToolMsg(msg.(*chat.ToolMessage))...)
		} else {
			result = append(result, r.buildMsg(msg))
		}
	}
	return result
}

func (r *requestHelper) buildApiChatRequest(req *chat.Request) (*anthropicsdk.MessageNewParams, error) {
	params, err := r.buildParams(req.Options)
	if err != nil {
		return nil, err
	}

	params.System = r.buildSystem(req.Messages)
	params.Messages = r.buildMsgs(req.Messages)

	return params, nil
}

type responseHelper struct{}

func (r *responseHelper) mapFinishReason(stopReason anthropicsdk.StopReason) chat.FinishReason {
	switch stopReason {
	case anthropicsdk.StopReasonEndTurn, anthropicsdk.StopReasonStopSequence:
		return chat.FinishReasonStop
	case anthropicsdk.StopReasonMaxTokens:
		return chat.FinishReasonLength
	case anthropicsdk.StopReasonToolUse:
		return chat.FinishReasonToolCalls
	case anthropicsdk.StopReasonRefusal:
		return chat.FinishReasonContentFilter
	default:
		return chat.FinishReasonOther
	}
}

func (r *responseHelper) buildAssistantMsg(resp *anthropicsdk.Message) *chat.AssistantMessage {
	msgParams := chat.MessageParams{
		Metadata: make(map[string]any),
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			msgParams.Text += block.Text
		case "tool_use":
			rawInput, _ := json.Marshal(block.Input)
			msgParams.ToolCalls = append(msgParams.ToolCalls, &chat.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(rawInput),
			})
		}
	}

	return chat.NewAssistantMessage(msgParams)
}

// buildThinkingResult constructs a thinking-typed Result from a single
// ThinkingBlock or RedactedThinkingBlock. The visible reasoning text (if
// any) goes into AssistantMessage.Text; the signature or redacted data is
// stored in metadata so the request side can replay it verbatim on the
// next turn (multi-Result pattern, see core/model/chat/thinking.go).
func (r *responseHelper) buildThinkingResult(block anthropicsdk.ContentBlockUnion, finish chat.FinishReason) (*chat.Result, error) {
	metadata := map[string]any{
		chat.MetaIsThought: true,
	}
	switch block.Type {
	case "thinking":
		metadata[chat.MetaThinkingSignature] = block.Signature
	case "redacted_thinking":
		metadata[chat.MetaRedactedThinkingData] = block.Data
	default:
		return nil, nil
	}
	msg := chat.NewAssistantMessage(chat.MessageParams{
		Text:     block.Thinking,
		Metadata: metadata,
	})
	return chat.NewResult(msg, &chat.ResultMetadata{FinishReason: finish})
}

// buildResults emits one Result per logical content kind. Thinking blocks
// (and redacted thinking blocks) become standalone Results carrying the
// MetaIsThought flag; text + tool_use are aggregated into a final Result.
// This is the Anthropic-side instance of Spring AI's multi-Generation
// pattern: the Response.Results list itself encodes block boundaries.
func (r *responseHelper) buildResults(resp *anthropicsdk.Message) ([]*chat.Result, error) {
	finish := r.mapFinishReason(resp.StopReason)
	results := make([]*chat.Result, 0, len(resp.Content))

	for _, block := range resp.Content {
		if block.Type != "thinking" && block.Type != "redacted_thinking" {
			continue
		}
		// Skip in-flight thinking deltas during streaming where the
		// signature has not yet arrived; those become valid Results once
		// the SDK accumulator has populated the signature.
		if block.Type == "thinking" && block.Signature == "" {
			continue
		}
		thinkingResult, err := r.buildThinkingResult(block, finish)
		if err != nil {
			return nil, err
		}
		if thinkingResult != nil {
			results = append(results, thinkingResult)
		}
	}

	mainMsg := r.buildAssistantMsg(resp)
	mainResult, err := chat.NewResult(mainMsg, &chat.ResultMetadata{FinishReason: finish})
	if err != nil {
		return nil, err
	}
	results = append(results, mainResult)

	return results, nil
}

func (r *responseHelper) buildMeta(req *anthropicsdk.MessageNewParams, resp *anthropicsdk.Message) *chat.ResponseMetadata {
	meta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
		Usage: &chat.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			OriginalUsage:    resp.Usage,
		},
	}
	meta.Set("original.request", req)
	meta.Set("original.response", resp)
	meta.Set("stop_sequence", resp.StopSequence)

	return meta
}

func (r *responseHelper) buildChatResponse(req *anthropicsdk.MessageNewParams, resp *anthropicsdk.Message) (*chat.Response, error) {
	results, err := r.buildResults(resp)
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(req, resp)

	return chat.NewResponse(results, meta)
}

type ChatModelConfig struct {
	ApiKey         model.ApiKey
	DefaultOptions *chat.Options
	RequestOptions []option.RequestOption
}

func (c *ChatModelConfig) validate() error {
	if c == nil {
		return errors.New("anthropic: config is nil")
	}
	if c.ApiKey == nil {
		return errors.New("anthropic: api key is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("anthropic: default options are required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *Api
	defaultOptions *chat.Options
	reqHelper      requestHelper
	respHelper     responseHelper
}

func NewChatModel(cfg *ChatModelConfig) (*ChatModel, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	api, err := NewApi(&ApiConfig{
		ApiKey:         cfg.ApiKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	return &ChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper: requestHelper{
			cfg.DefaultOptions,
		},
	}, nil
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	apiReq, err := c.reqHelper.buildApiChatRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	return c.respHelper.buildChatResponse(apiReq, apiResp)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		apiReq, err := c.reqHelper.buildApiChatRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiStream := c.api.ChatCompletionStream(ctx, apiReq)
		defer apiStream.Close()

		acc := anthropicsdk.Message{}

		for apiStream.Next() {
			event := apiStream.Current()

			if err = acc.Accumulate(event); err != nil {
				yield(nil, err)
				return
			}

			if acc.ID == "" {
				continue
			}

			resp, err := c.respHelper.buildChatResponse(apiReq, &acc)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}

		if err = apiStream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (c *ChatModel) DefaultOptions() *chat.Options {
	return c.defaultOptions
}

func (c *ChatModel) Info() chat.ModelInfo {
	return chat.ModelInfo{
		Provider: Provider,
	}
}
