package openai

import (
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/media"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/pkg/mime"
)

type chatHelper struct {
	reqHelper  requestHelper
	respHelper responseHelper
}

func newChatHelper(defaultOpts *chat.Options) chatHelper {
	return chatHelper{
		reqHelper: requestHelper{
			defaultOpts,
		},
	}
}

func (h *chatHelper) buildChatRequest(req *chat.Request) (*openai.ChatCompletionNewParams, error) {
	return h.reqHelper.buildRequest(req)
}

func (h *chatHelper) buildChatResponse(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) (*chat.Response, error) {
	return h.respHelper.buildResponse(req, resp)
}

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
	params := new(openai.ChatCompletionNewParams)

	if extra, exist := opts.Get(ChatCompletionOptions); exist && extra != nil {
		if extraParams, ok := extra.(*openai.ChatCompletionNewParams); ok {
			params = extraParams
		}
	}

	params.Model = opts.Model

	if opts.FrequencyPenalty != nil {
		params.FrequencyPenalty = openai.Float(*opts.FrequencyPenalty)
	}
	if opts.MaxTokens != nil {
		params.MaxTokens = openai.Int(*opts.MaxTokens)
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

func (r *requestHelper) buildParams(opts *chat.Options) (*openai.ChatCompletionNewParams, error) {
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
			part.OfImageURL = &openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
					URL: data,
				},
			}
		} else if mime.IsAudio(mt) {
			part.OfInputAudio = &openai.ChatCompletionContentPartInputAudioParam{
				InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
					Data:   data,
					Format: mt.SubType(),
				},
			}
		} else {
			part.OfFile = &openai.ChatCompletionContentPartFileParam{
				File: openai.ChatCompletionContentPartFileFileParam{
					FileData: openai.String(data),
					Filename: openai.String(md.Name),
					FileID:   openai.String(md.ID),
				},
			}
		}

		parts = append(parts, part)
	}

	return openai.UserMessage(parts)
}

func (r *requestHelper) buildAssistantMsg(msg *chat.AssistantMessage) openai.ChatCompletionMessageParamUnion {
	message := openai.AssistantMessage(msg.Text)
	assistant := message.OfAssistant

	for _, tc := range msg.ToolCalls {
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

	for _, md := range msg.Media {
		if mime.IsAudio(md.MimeType) {
			assistant.Audio.ID = md.ID
			break // only one is allowed
		}
	}

	if refusal, exists := msg.Metadata["refusal"]; exists {
		assistant.Refusal = openai.String(cast.ToString(refusal))
	}

	return message
}

func (r *requestHelper) buildToolMsgs(msg *chat.ToolMessage) []openai.ChatCompletionMessageParamUnion {
	returns := msg.ToolReturns
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(returns))

	for _, ret := range returns {
		msgs = append(msgs, openai.ToolMessage(ret.Result, ret.ID))
	}

	return msgs
}

func (r *requestHelper) buildMsg(msg chat.Message) openai.ChatCompletionMessageParamUnion {
	if msg.Type().IsUser() {
		return r.buildUserMsg(msg.(*chat.UserMessage))
	} else if msg.Type().IsAssistant() {
		return r.buildAssistantMsg(msg.(*chat.AssistantMessage))
	} else {
		return r.buildSystemMsg(msg.(*chat.SystemMessage))
	}
}

func (r *requestHelper) buildMsgs(msgs []chat.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		if msg.Type().IsTool() {
			result = append(result, r.buildToolMsgs(msg.(*chat.ToolMessage))...)
		} else {
			result = append(result, r.buildMsg(msg))
		}
	}

	return result
}

func (r *requestHelper) buildRequest(req *chat.Request) (*openai.ChatCompletionNewParams, error) {
	params, err := r.buildParams(req.Options)
	if err != nil {
		return nil, err
	}

	params.Messages = r.buildMsgs(req.Messages)
	return params, nil
}

type responseHelper struct{}

func (r *responseHelper) buildAssistantMsg(req *openai.ChatCompletionNewParams, msg *openai.ChatCompletionMessage) *chat.AssistantMessage {
	param := chat.MessageParams{
		Text:     msg.Content,
		Metadata: make(map[string]any),
	}
	param.Metadata["refusal"] = msg.Refusal
	param.Metadata["annotations"] = msg.Annotations

	for _, tc := range msg.ToolCalls {
		param.ToolCalls = append(param.ToolCalls, &chat.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	if msg.Audio.ID != "" {
		param.Metadata["audio.id"] = msg.Audio.ID
		param.Metadata["audio.expires_at"] = msg.Audio.ExpiresAt

		mt, _ := mime.New("audio", string(req.Audio.Format))
		param.Metadata["audio.mimetype"] = mt.String()
		param.Metadata["audio.voice"] = req.Audio.Voice

		param.Media = append(param.Media, &media.Media{
			ID:       msg.Audio.ID,
			MimeType: mt,
			Data:     msg.Audio.Data,
		})

		if param.Text == "" {
			param.Text = msg.Audio.Transcript
		}
	}

	return chat.NewAssistantMessage(param)
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

func (r *responseHelper) buildResults(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) ([]*chat.Result, error) {
	results := make([]*chat.Result, 0, len(resp.Choices))

	for _, choice := range resp.Choices {
		result, err := r.buildResult(req, &choice)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *responseHelper) buildMeta(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) *chat.ResponseMetadata {
	meta := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: resp.Created,
		Usage: &chat.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			OriginalUsage:    resp.Usage,
		},
	}
	meta.Set("original.request", req)
	meta.Set("original.response", resp)
	meta.Set("service_tier", resp.ServiceTier)
	meta.Set("system_fingerprint", resp.SystemFingerprint)

	return meta
}

func (r *responseHelper) buildResponse(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) (*chat.Response, error) {
	results, err := r.buildResults(req, resp)
	if err != nil {
		return nil, err
	}

	meta := r.buildMeta(req, resp)

	return chat.NewResponse(results, meta)
}
