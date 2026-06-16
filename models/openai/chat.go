package openai

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/internal/catalog"
	"github.com/Tangerg/lynx/models/internal/options"
	"github.com/Tangerg/lynx/pkg/mime"
)

type requestHelper struct {
	defaultOptions *chat.Options
}

func (r *requestHelper) buildToolParams(tools []chat.Tool) ([]openai.ChatCompletionToolUnionParam, error) {
	toolParams := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))

	for _, t := range tools {
		var params map[string]any
		def := t.Definition()

		if err := json.Unmarshal([]byte(def.InputSchema), &params); err != nil {
			return nil, err
		}

		toolParams = append(toolParams, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        def.Name,
					Description: openai.String(def.Description),
					Strict:      openai.Bool(true),
					Parameters:  params,
				},
			},
		})
	}

	return toolParams, nil
}

func (r *requestHelper) buildBaseParams(opts *chat.Options) *openai.ChatCompletionNewParams {
	params := options.GetParams[openai.ChatCompletionNewParams](opts, OptionsKey)

	params.Model = opts.Model

	if opts.FrequencyPenalty != nil {
		params.FrequencyPenalty = openai.Float(*opts.FrequencyPenalty)
	}
	if opts.MaxTokens != nil {
		// max_tokens is rejected by o-series and gpt-5 models; route to
		// the replacement field. Callers needing the legacy field for a
		// specific old model set it via Extra-threaded params.
		params.MaxCompletionTokens = openai.Int(*opts.MaxTokens)
	}
	if opts.PresencePenalty != nil {
		params.PresencePenalty = openai.Float(*opts.PresencePenalty)
	}
	if len(opts.Stop) > 0 {
		params.Stop.OfStringArray = opts.Stop
	}
	if opts.Temperature != nil {
		params.Temperature = openai.Float(*opts.Temperature)
	}
	if opts.TopP != nil {
		params.TopP = openai.Float(*opts.TopP)
	}

	return params
}

func (r *requestHelper) buildParams(opts *chat.Options, tools []chat.Tool) (*openai.ChatCompletionNewParams, error) {
	mergedOpts, err := chat.MergeOptions(r.defaultOptions, opts)
	if err != nil {
		return nil, err
	}

	params := r.buildBaseParams(mergedOpts)

	params.Tools, err = r.buildToolParams(tools)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func (r *requestHelper) buildSystemMsg(msg *chat.SystemMessage) openai.ChatCompletionMessageParamUnion {
	return openai.SystemMessage(msg.Text)
}

func (r *requestHelper) buildUserMsg(msg *chat.UserMessage) openai.ChatCompletionMessageParamUnion {
	if !msg.HasMedia() {
		return openai.UserMessage(msg.Text)
	}

	parts := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(msg.Media))
	parts = append(parts, openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: msg.Text,
		},
	})

	for _, md := range msg.Media {
		data, err := md.DataAsString()
		if err != nil {
			continue
		}

		part := openai.ChatCompletionContentPartUnionParam{}
		mt := md.MimeType

		if mime.IsImage(mt) {
			// image_url.url requires a URL — an http(s)/data: URL is passed
			// through, raw base64 is wrapped into a data URL. The OpenAI API
			// rejects bare base64 here (per the vision guide).
			part.OfImageURL = &openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
					URL: mediaDataURL(mt, data),
				},
			}
		} else if mime.IsAudio(mt) {
			// input_audio is the exception: it takes RAW base64 with a separate
			// format field — no data: prefix.
			part.OfInputAudio = &openai.ChatCompletionContentPartInputAudioParam{
				InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
					Data:   data,
					Format: mt.SubType(),
				},
			}
		} else {
			// file_data, like image_url.url, is a data URL.
			part.OfFile = &openai.ChatCompletionContentPartFileParam{
				File: openai.ChatCompletionContentPartFileFileParam{
					FileData: openai.String(mediaDataURL(mt, data)),
					Filename: openai.String(md.Name),
					FileID:   openai.String(md.ID),
				},
			}
		}

		parts = append(parts, part)
	}

	return openai.UserMessage(parts)
}

// mediaDataURL renders a media payload for the OpenAI parts whose field is a
// URL (image_url.url, file file_data): an http(s)/data: URL passes through, and
// raw base64 is wrapped into a `data:<mime>;base64,<b64>` data URL. The OpenAI
// API rejects bare base64 in these fields — unlike input_audio.data, which
// takes raw base64 with a separate format field. mt nil (unknown type) returns
// data unchanged, since a data URL can't be formed without a media type.
func mediaDataURL(mt *mime.MIME, data string) string {
	if mt == nil ||
		strings.HasPrefix(data, "data:") ||
		strings.HasPrefix(data, "http://") ||
		strings.HasPrefix(data, "https://") {
		return data
	}
	return "data:" + mt.TypeAndSubType() + ";base64," + data
}

func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) openai.ChatCompletionMessageParamUnion {
	// Parts → flat (content + tool_calls). The OpenAI Chat Completions
	// API does not support interleaved text/tool_use blocks; all
	// TextParts are projected into a single content string (in order) and
	// tool_calls as a separate array. ReasoningParts are dropped here —
	// the API doesn't accept reasoning content on the request side.
	message := openai.AssistantMessage(msg.JoinedText())
	assistant := message.OfAssistant

	for tc := range msg.ToolCalls() {
		assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: tc.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			},
		})
	}

	if audioID, ok := msg.Metadata["audio.id"].(string); ok && audioID != "" {
		assistant.Audio.ID = audioID
	}

	if refusal, exists := msg.Metadata["refusal"]; exists {
		assistant.Refusal = openai.String(cast.ToString(refusal))
	}

	return message
}

func (r *requestHelper) buildToolMsg(msg *chat.ToolMessage) []openai.ChatCompletionMessageParamUnion {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(msg.ToolReturns))

	for _, ret := range msg.ToolReturns {
		msgs = append(msgs, openai.ToolMessage(ret.Result, ret.ID))
	}

	return msgs
}

func (r *requestHelper) buildMsg(msg chat.Message) openai.ChatCompletionMessageParamUnion {
	if msg.Type() == chat.MessageTypeUser {
		return r.buildUserMsg(msg.(*chat.UserMessage))
	}
	if msg.Type() == chat.MessageTypeAssistant {
		return r.buildAssistantMsg(msg.(*chat.AssistantMessage))
	}
	return r.buildSystemMsg(msg.(*chat.SystemMessage))
}

func (r *requestHelper) buildMsgs(msgs []chat.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		if msg.Type() == chat.MessageTypeTool {
			result = append(result, r.buildToolMsg(msg.(*chat.ToolMessage))...)
		} else {
			result = append(result, r.buildMsg(msg))
		}
	}

	return result
}

func (r *requestHelper) buildAPIChatRequest(req *chat.Request) (*openai.ChatCompletionNewParams, error) {
	params, err := r.buildParams(req.Options, req.Tools)
	if err != nil {
		return nil, err
	}

	params.Messages = r.buildMsgs(req.Messages)
	return params, nil
}

type responseHelper struct{}

func (r *responseHelper) buildAssistantMsg(req *openai.ChatCompletionNewParams, msg *openai.ChatCompletionMessage) *chat.AssistantMessage {
	// OpenAI Chat Completions splits content (string) and tool_calls
	// into separate fields — there's no interleaved ordering on the
	// wire. We rebuild a Parts sequence by convention: reasoning first
	// (if any), then text, then tool_calls. This is the canonical
	// "first text then tools" ordering OpenAI emits in practice.
	parts := make([]chat.OutputPart, 0, 2+len(msg.ToolCalls))

	// DeepSeek-R1, vLLM with reasoning parsers, and other OpenAI-compatible
	// servers expose chain-of-thought as a non-standard `reasoning_content`
	// field on the message. The official openai-go types do not surface
	// it, but the raw JSON is preserved under JSON.ExtraFields.
	if reasoningField, ok := msg.JSON.ExtraFields["reasoning_content"]; ok && reasoningField.Valid() {
		var reasoning string
		if err := json.Unmarshal([]byte(reasoningField.Raw()), &reasoning); err == nil && reasoning != "" {
			parts = append(parts, &chat.ReasoningPart{Text: reasoning})
		}
	}

	if msg.Content != "" {
		parts = append(parts, &chat.TextPart{Text: msg.Content})
	}

	for _, tc := range msg.ToolCalls {
		parts = append(parts, &chat.ToolCallPart{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	msgParams := chat.MessageParams{
		Parts:    parts,
		Metadata: make(map[string]any),
	}
	msgParams.Metadata["refusal"] = msg.Refusal
	msgParams.Metadata["annotations"] = msg.Annotations

	if msg.Audio.ID != "" {
		msgParams.Metadata["audio.id"] = msg.Audio.ID
		msgParams.Metadata["audio.expires_at"] = msg.Audio.ExpiresAt

		mt, _ := mime.New("audio", string(req.Audio.Format))
		msgParams.Metadata["audio.mimetype"] = mt.String()
		msgParams.Metadata["audio.voice"] = req.Audio.Voice
		msgParams.Metadata["audio.data"] = msg.Audio.Data

		// Audio-output mode: surface the transcript as a TextPart when
		// no other text was produced, so callers reading JoinedText()
		// still get the spoken content. Media is preserved on the
		// AssistantMessage by routing through Metadata under
		// "audio.data" / "audio.mimetype" — there is no top-level
		// Media slice on the Parts-based AssistantMessage in v1.
		hasText := slices.ContainsFunc(msgParams.Parts, func(p chat.OutputPart) bool {
			tp, ok := p.(*chat.TextPart)
			return ok && tp.Text != ""
		})
		if msg.Audio.Transcript != "" && !hasText {
			msgParams.Parts = append(msgParams.Parts, &chat.TextPart{Text: msg.Audio.Transcript})
		}
	}

	return chat.NewAssistantMessage(msgParams)
}

func (r *responseHelper) buildResultMeta(req *openai.ChatCompletionNewParams, choice *openai.ChatCompletionChoice) *chat.ResultMetadata {
	meta := &chat.ResultMetadata{
		FinishReason: chat.FinishReason(choice.FinishReason),
	}
	meta.Set("index", choice.Index)

	if req.Logprobs.Value {
		meta.Set("logprobs", choice.Logprobs)
	}

	return meta
}

func (r *responseHelper) buildResult(req *openai.ChatCompletionNewParams, choice *openai.ChatCompletionChoice) (*chat.Result, error) {
	assistantMsg := r.buildAssistantMsg(req, &choice.Message)
	meta := r.buildResultMeta(req, choice)

	return chat.NewResult(assistantMsg, meta)
}

func (r *responseHelper) buildMeta(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) *chat.ResponseMetadata {
	usage := &chat.Usage{
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		OriginalUsage:    resp.Usage,
	}
	// Surface o-series reasoning token breakdown when present. The SDK
	// returns 0 when the field is absent from the response payload, so a
	// 0 value is indistinguishable from "not exposed"; any non-zero
	// count is treated as an explicit signal worth surfacing.
	if rt := resp.Usage.CompletionTokensDetails.ReasoningTokens; rt > 0 {
		usage.ReasoningTokens = &rt
	}
	// Surface OpenAI prompt-cache hits (prompt_tokens_details.cached_tokens).
	// OpenAI's caching is automatic with no separate write-side billing,
	// so only CacheReadInputTokens is populated here.
	if ct := resp.Usage.PromptTokensDetails.CachedTokens; ct > 0 {
		usage.CacheReadInputTokens = &ct
	}
	meta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: resp.Created,
		Usage:   usage,
	}
	meta.Set("service_tier", resp.ServiceTier)

	return meta
}

func (r *responseHelper) buildChatResponse(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) (*chat.Response, error) {
	// OpenAI returns Choices[] sized by params.N (default 1). The chat
	// surface is single-completion by design — see chat.Response — so we
	// take Choices[0] and ignore extras. Callers needing N>1 should drop
	// down to the openai-go SDK directly.
	if len(resp.Choices) == 0 {
		return nil, errors.New("openai: response has no choices")
	}

	result, err := r.buildResult(req, &resp.Choices[0])
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(req, resp)

	return chat.NewResponse(result, meta)
}

type ChatModelConfig struct {
	APIKey         model.APIKey
	DefaultOptions *chat.Options
	RequestOptions []option.RequestOption

	// Metadata overrides the [chat.ModelMetadata] returned by [ChatModel.Metadata].
	// Facades over this package (deepseek, alibaba, zhipu, ...) pass
	// their own Provider here so observability tags the call by the
	// real upstream brand, not by "OpenAI". Zero Provider falls back
	// to the package default [Provider].
	Metadata *chat.ModelMetadata
}

func (c ChatModelConfig) Validate() error {
	if c.APIKey == nil {
		return errors.New("openai: APIKey is required")
	}
	if c.DefaultOptions == nil {
		return errors.New("openai: DefaultOptions is required")
	}
	return nil
}

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *API
	defaultOptions *chat.Options
	reqHelper      requestHelper
	respHelper     responseHelper
	metadata       chat.ModelMetadata
}

func NewChatModel(cfg ChatModelConfig) (*ChatModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	api, err := NewAPI(APIConfig{
		APIKey:         cfg.APIKey,
		RequestOptions: cfg.RequestOptions,
	})
	if err != nil {
		return nil, err
	}

	info := catalog.Resolve(Provider, cfg.DefaultOptions, cfg.Metadata)
	return &ChatModel{
		api:            api,
		defaultOptions: cfg.DefaultOptions,
		reqHelper: requestHelper{
			cfg.DefaultOptions,
		},
		metadata: info,
	}, nil
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	apiReq, err := c.reqHelper.buildAPIChatRequest(req)
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
		apiReq, err := c.reqHelper.buildAPIChatRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}

		apiStream, err := c.api.ChatCompletionStream(ctx, apiReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer apiStream.Close()

		// Per-chunk accumulator: spin up a fresh one each iteration so
		// the resulting ChatCompletion reflects ONLY this chunk's
		// delta content. The SDK accumulator gives us a non-streaming
		// ChatCompletion shape that feeds into the existing
		// buildChatResponse, while delta semantics are preserved for
		// the upstream chat.ResponseAccumulator (which does the
		// stream-wide stitching).
		for apiStream.Next() {
			acc := openai.ChatCompletionAccumulator{}
			acc.AddChunk(apiStream.Current())

			resp, err := c.respHelper.buildChatResponse(apiReq, &acc.ChatCompletion)
			if err != nil {
				yield(nil, err)
				return
			}

			if !yield(resp, nil) {
				return
			}
		}

		if err := apiStream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return *c.defaultOptions
}

func (c *ChatModel) Metadata() chat.ModelMetadata {
	return c.metadata
}
