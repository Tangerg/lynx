package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
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
	blocks := make([]anthropicsdk.ContentBlockParamUnion, 0, 2+len(msg.ToolCalls))

	// Anthropic requires content blocks in order: thinking → text → tool_use.
	// Redacted thinking and standard thinking can both appear before text;
	// signatures (when present) must be echoed verbatim so the API can
	// validate that prior reasoning has not been tampered with.
	if data := redactedReasoning(msg); data != "" {
		blocks = append(blocks, anthropicsdk.NewRedactedThinkingBlock(data))
	}
	sig := reasoningSignature(msg)
	if msg.Reasoning != "" && sig != "" {
		blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, msg.Reasoning))
	} else if sig != "" {
		// OMITTED display mode: signature only, no visible reasoning text.
		// Anthropic accepts an empty thinking string with a non-empty
		// signature in this mode.
		blocks = append(blocks, anthropicsdk.NewThinkingBlock(sig, ""))
	}
	// If we have Reasoning text but no signature, drop it on replay — the
	// signature is required for the API to accept the block, and a missing
	// signature usually means the message arrived from an external source
	// that did not preserve the continuation token.

	if msg.Text != "" {
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

// buildAssistantMsg folds all ContentBlocks of an Anthropic response into
// a single AssistantMessage. Thinking text accumulates into Reasoning;
// the first non-empty signature is stored under MetaReasoningSignature
// (sufficient for standard extended thinking — interleaved-thinking beta
// is not supported in this initial implementation). Redacted thinking
// data is stored under MetaRedactedReasoning. Tool uses become ToolCalls.
func (r *responseHelper) buildAssistantMsg(resp *anthropicsdk.Message) *chat.AssistantMessage {
	msgParams := chat.MessageParams{
		Metadata: make(map[string]any),
	}

	var reasoningBuf strings.Builder
	var firstSignature string
	var redactedData string

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
		case "thinking":
			reasoningBuf.WriteString(block.Thinking)
			if firstSignature == "" {
				firstSignature = block.Signature
			}
		case "redacted_thinking":
			redactedData = block.Data
		}
	}

	msgParams.Reasoning = reasoningBuf.String()
	if firstSignature != "" {
		msgParams.Metadata[MetaReasoningSignature] = firstSignature
	}
	if redactedData != "" {
		msgParams.Metadata[MetaRedactedReasoning] = redactedData
	}

	return chat.NewAssistantMessage(msgParams)
}

func (r *responseHelper) buildResult(resp *anthropicsdk.Message) (*chat.Result, error) {
	msg := r.buildAssistantMsg(resp)
	return chat.NewResult(msg, &chat.ResultMetadata{
		FinishReason: r.mapFinishReason(resp.StopReason),
	})
}

func (r *responseHelper) buildMeta(req *anthropicsdk.MessageNewParams, resp *anthropicsdk.Message) *chat.ResponseMetadata {
	usage := &chat.Usage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		OriginalUsage:    resp.Usage,
	}
	// Surface Anthropic's prompt-cache breakdown when ephemeral caching
	// is in use. The SDK returns 0 when the field is absent from the
	// response payload, so a 0 value is indistinguishable from "the
	// provider did not surface this dimension"; we treat any non-zero
	// count as an explicit signal worth surfacing. Both fields are
	// subsets of InputTokens (= PromptTokens above).
	if v := resp.Usage.CacheReadInputTokens; v > 0 {
		usage.CacheReadInputTokens = &v
	}
	if v := resp.Usage.CacheCreationInputTokens; v > 0 {
		usage.CacheWriteInputTokens = &v
	}

	meta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: time.Now().Unix(),
		Usage:   usage,
	}
	meta.Set("original.request", req)
	meta.Set("original.response", resp)
	meta.Set("stop_sequence", resp.StopSequence)

	return meta
}

func (r *responseHelper) buildChatResponse(req *anthropicsdk.MessageNewParams, resp *anthropicsdk.Message) (*chat.Response, error) {
	result, err := r.buildResult(resp)
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(req, resp)

	return chat.NewResponse([]*chat.Result{result}, meta)
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
