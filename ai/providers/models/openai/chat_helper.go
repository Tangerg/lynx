package openai

import (
	"encoding/json"

	"github.com/openai/openai-go"
	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/commons/content"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/mime"
)

type chatHelper struct {
	requestHelper  requestHelper
	responseHelper responseHelper
}

func newChatHelper(defaultOptions *ChatOptions) chatHelper {
	return chatHelper{
		requestHelper: requestHelper{
			defaultOptions,
		},
	}
}

func (h *chatHelper) makeApiChatCompletionRequest(req *chat.Request) (*openai.ChatCompletionNewParams, error) {
	return h.requestHelper.makeRequest(req)
}

func (h *chatHelper) makeChatResponse(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) (*chat.Response, error) {
	return h.responseHelper.makeResponse(req, resp)
}

type requestHelper struct {
	defaultOptions *ChatOptions
}

func (r *requestHelper) makeParamsTools(tools []tool.Tool) ([]openai.ChatCompletionToolParam, error) {
	rv := make([]openai.ChatCompletionToolParam, 0, len(tools))

	for _, t := range tools {
		var (
			parameters map[string]any
			definition = t.Definition()
		)

		err := json.Unmarshal([]byte(definition.InputSchema()), &parameters)
		if err != nil {
			return nil, err
		}

		rv = append(
			rv,
			openai.ChatCompletionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        definition.Name(),
					Description: openai.String(definition.Description()),
					Strict:      openai.Bool(true),
					Parameters:  parameters,
				},
			},
		)
	}

	return rv, nil
}

func (r *requestHelper) makeParamsBase(chatOptions *ChatOptions) *openai.ChatCompletionNewParams {
	params := new(openai.ChatCompletionNewParams)
	params.Model = chatOptions.model

	if chatOptions.frequencyPenalty != nil {
		params.FrequencyPenalty = openai.Float(*chatOptions.frequencyPenalty)
	}
	if chatOptions.logitBias != nil {
		params.LogitBias = chatOptions.logitBias
	}
	if chatOptions.logprobs != nil {
		params.Logprobs = openai.Bool(*chatOptions.logprobs)
	}
	if chatOptions.maxCompletionTokens != nil {
		params.MaxCompletionTokens = openai.Int(*chatOptions.maxCompletionTokens)
	}
	if chatOptions.maxTokens != nil {
		params.MaxTokens = openai.Int(*chatOptions.maxTokens)
	}
	if chatOptions.metadata != nil {
		params.Metadata = chatOptions.metadata
	}
	if chatOptions.modalities != nil {
		params.Modalities = chatOptions.modalities
	}
	if chatOptions.n != nil {
		params.N = openai.Int(*chatOptions.n)
	}
	if chatOptions.parallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*chatOptions.parallelToolCalls)
	}
	if chatOptions.presencePenalty != nil {
		params.PresencePenalty = openai.Float(*chatOptions.presencePenalty)
	}
	if chatOptions.reasoningEffort != nil {
		params.ReasoningEffort = openai.ReasoningEffort(*chatOptions.reasoningEffort)
	}
	if chatOptions.seed != nil {
		params.Seed = openai.Int(*chatOptions.seed)
	}
	if chatOptions.serviceTier != nil {
		params.ServiceTier = openai.ChatCompletionNewParamsServiceTier(*chatOptions.serviceTier)
	}
	if len(chatOptions.stop) > 0 {
		params.Stop.OfStringArray = chatOptions.stop
	}
	if chatOptions.store != nil {
		params.Store = openai.Bool(*chatOptions.store)
	}
	if chatOptions.temperature != nil {
		params.Temperature = openai.Float(*chatOptions.temperature)
	}
	if chatOptions.topLogprobs != nil {
		params.TopLogprobs = openai.Int(*chatOptions.topLogprobs)
	}
	if chatOptions.topP != nil {
		params.TopP = openai.Float(*chatOptions.topP)
	}
	if chatOptions.user != nil {
		params.User = openai.String(*chatOptions.user)
	}

	return params
}

func (r *requestHelper) makeParams(options chat.Options) (*openai.ChatCompletionNewParams, error) {
	chatOptions, err := MergeChatOptions(r.defaultOptions, options)
	if err != nil {
		return nil, err
	}

	params := r.makeParamsBase(chatOptions)
	params.Tools, err = r.makeParamsTools(chatOptions.tools)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func (r *requestHelper) makeSystemMessage(msg *messages.SystemMessage) openai.ChatCompletionMessageParamUnion {
	return openai.SystemMessage(msg.Text())
}

func (r *requestHelper) makeUserMessage(msg *messages.UserMessage) openai.ChatCompletionMessageParamUnion {
	if !msg.HasMedia() {
		return openai.UserMessage(msg.Text())
	}

	params := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(msg.Media()))
	params = append(params, openai.ChatCompletionContentPartUnionParam{
		OfText: &openai.ChatCompletionContentPartTextParam{
			Text: msg.Text(),
		},
	})

	for _, media := range msg.Media() {
		mt := media.MimeType()
		data, err := media.DataAsString()
		if err != nil {
			continue
		}

		param := openai.ChatCompletionContentPartUnionParam{}
		if mime.IsImage(mt) {
			param.OfImageURL = &openai.ChatCompletionContentPartImageParam{
				ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
					URL: data,
				},
			}
		} else if mime.IsAudio(mt) {
			param.OfInputAudio = &openai.ChatCompletionContentPartInputAudioParam{
				InputAudio: openai.ChatCompletionContentPartInputAudioInputAudioParam{
					Data:   data,
					Format: mt.SubType(),
				},
			}
		} else {
			param.OfFile = &openai.ChatCompletionContentPartFileParam{
				File: openai.ChatCompletionContentPartFileFileParam{
					FileData: openai.String(data),
					Filename: openai.String(media.Name()),
					FileID:   openai.String(media.ID()),
				},
			}
		}
		params = append(params, param)
	}

	return openai.UserMessage(params)
}

func (r *requestHelper) makeAssistantMessage(msg *messages.AssistantMessage) openai.ChatCompletionMessageParamUnion {
	message := openai.AssistantMessage(msg.Text())
	ofAssistant := message.OfAssistant

	for _, toolCall := range msg.ToolCalls() {
		ofAssistant.ToolCalls = append(
			ofAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
				ID: toolCall.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
	}

	for _, media := range msg.Media() {
		if mime.IsAudio(media.MimeType()) {
			ofAssistant.Audio.ID = media.ID()
			//only one is allowed
			break
		}
	}

	for k, v := range msg.Metadata() {
		if k == "refusal" {
			ofAssistant.Refusal = openai.String(cast.ToString(v))
		}
	}

	return message
}

func (r *requestHelper) makeToolMessages(msg *messages.ToolResponseMessage) []openai.ChatCompletionMessageParamUnion {
	toolResponses := msg.ToolResponses()
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(toolResponses))

	for _, toolResponse := range toolResponses {
		msgs = append(msgs, openai.ToolMessage(toolResponse.ResponseData, toolResponse.ID))
	}

	return msgs
}

// makeMessage All assertions cannot panic because the parameters have already been determined during construction by [chat.NewRequest] and [messages.FilterStandardTypes]
func (r *requestHelper) makeMessage(msg messages.Message) openai.ChatCompletionMessageParamUnion {
	if msg.Type().IsSystem() {
		return r.makeSystemMessage(msg.(*messages.SystemMessage))
	}
	if msg.Type().IsUser() {
		return r.makeUserMessage(msg.(*messages.UserMessage))
	}
	if msg.Type().IsAssistant() {
		return r.makeAssistantMessage(msg.(*messages.AssistantMessage))
	}

	return openai.UserMessage(msg.Text())
}

func (r *requestHelper) makeMessages(msgs []messages.Message) []openai.ChatCompletionMessageParamUnion {
	rv := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))

	for _, msg := range msgs {
		if msg.Type().IsTool() {
			rv = append(rv, r.makeToolMessages(msg.(*messages.ToolResponseMessage))...)
		} else {
			rv = append(rv, r.makeMessage(msg))
		}
	}

	return rv
}

func (r *requestHelper) makeRequest(req *chat.Request) (*openai.ChatCompletionNewParams, error) {
	params, err := r.makeParams(req.Options())
	if err != nil {
		return nil, err
	}

	params.Messages = r.makeMessages(req.Instructions())
	return params, nil
}

type responseHelper struct{}

func (r *responseHelper) makeResultAssistantMessage(req *openai.ChatCompletionNewParams, message *openai.ChatCompletionMessage) *messages.AssistantMessage {
	param := messages.AssistantMessageParam{
		Text:     message.Content,
		Metadata: make(map[string]any),
	}
	param.Metadata["refusal"] = message.Refusal
	param.Metadata["annotations"] = message.Annotations

	for _, toolCall := range message.ToolCalls {
		param.ToolCalls = append(
			param.ToolCalls,
			&messages.ToolCall{
				ID:        toolCall.ID,
				Type:      "function",
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			},
		)
	}

	if message.Audio.ID != "" {
		param.Metadata["audio.id"] = message.Audio.ID
		param.Metadata["audio.expires_at"] = message.Audio.ExpiresAt
		mt, _ := mime.New("audio", string(req.Audio.Format))
		param.Metadata["audio.mimetype"] = mt.String()
		param.Metadata["audio.voice"] = req.Audio.Voice
		param.Media = append(
			param.Media,
			content.
				NewMediaBuilder().
				WithID(message.Audio.ID).
				WithData(message.Audio.Data).
				WithMimeType(mt).
				MustBuild(),
		)
		if param.Text == "" {
			param.Text = message.Audio.Transcript
		}
	}

	return messages.NewAssistantMessage(param)
}

func (r *responseHelper) makeResultMetadata(req *openai.ChatCompletionNewParams, choice *openai.ChatCompletionChoice) *chat.ResultMetadata {
	metadata := &chat.ResultMetadata{
		FinishReason: chat.FinishReason(choice.FinishReason),
	}
	metadata.Set("index", choice.Index)

	if req.Logprobs.Value {
		metadata.Set("logprobs", choice.Logprobs)
	}

	return metadata
}

func (r *responseHelper) makeResult(req *openai.ChatCompletionNewParams, choice *openai.ChatCompletionChoice) (*chat.Result, error) {
	assistantMessage := r.makeResultAssistantMessage(req, &choice.Message)
	metadata := r.makeResultMetadata(req, choice)

	return chat.NewResult(assistantMessage, metadata)
}

func (r *responseHelper) makeResults(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) ([]*chat.Result, error) {
	results := make([]*chat.Result, 0, len(resp.Choices))

	for _, choice := range resp.Choices {
		result, err := r.makeResult(req, &choice)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *responseHelper) makeMetadata(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) *chat.ResponseMetadata {
	metadata := &chat.ResponseMetadata{
		ID:      resp.ID,
		Model:   resp.Model,
		Created: resp.Created,
		Usage: &chat.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			OriginalUsage:    resp.Usage,
		},
	}
	metadata.Set("original.request", req)
	metadata.Set("original.response", resp)
	metadata.Set("service_tier", resp.ServiceTier)
	metadata.Set("system_fingerprint", resp.SystemFingerprint)

	return metadata
}

func (r *responseHelper) makeResponse(req *openai.ChatCompletionNewParams, resp *openai.ChatCompletion) (*chat.Response, error) {
	results, err := r.makeResults(req, resp)
	if err != nil {
		return nil, err
	}

	metadata := r.makeMetadata(req, resp)

	return chat.NewResponse(results, metadata)
}
