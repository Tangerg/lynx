package openaiv2

import (
	"encoding/json"

	"github.com/openai/openai-go"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
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

func (h *chatHelper) makeApiChatCompletionRequest(req *chat.Request) *openai.ChatCompletionNewParams {
	return h.requestHelper.makeRequest(req)
}

func (h *chatHelper) makeChatResponse(resp *openai.ChatCompletion) (*chat.Response, error) {
	return h.responseHelper.makeResponse(resp)
}

type requestHelper struct {
	defaultOptions *ChatOptions
}

func (r *requestHelper) makeOptionsTools(tools []tool.Tool) []openai.ChatCompletionToolParam {
	rv := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		var parameters map[string]any
		_ = json.Unmarshal([]byte(t.Definition().InputSchema()), &parameters)
		rv = append(rv, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        t.Definition().Name(),
				Description: openai.String(t.Definition().Description()),
				Strict:      openai.Bool(true),
				Parameters:  parameters,
			},
		})
	}
	return rv
}

func (r *requestHelper) makeOptions(options chat.Options) *openai.ChatCompletionNewParams {
	chatOptions, _ := MergeChatOptions(r.defaultOptions, options)

	opt := new(openai.ChatCompletionNewParams)
	opt.Model = chatOptions.model
	if chatOptions.frequencyPenalty != nil {
		opt.FrequencyPenalty = openai.Float(*chatOptions.frequencyPenalty)
	}
	if chatOptions.maxTokens != nil {
		opt.MaxTokens = openai.Int(*chatOptions.maxTokens)
	}
	if chatOptions.presencePenalty != nil {
		opt.PresencePenalty = openai.Float(*chatOptions.presencePenalty)
	}
	if len(chatOptions.stopSequences) > 0 {
		opt.Stop.OfStringArray = chatOptions.stopSequences
	}
	if chatOptions.temperature != nil {
		opt.Temperature = openai.Float(*chatOptions.temperature)
	}
	if chatOptions.topP != nil {
		opt.TopP = openai.Float(*chatOptions.topP)
	}
	opt.Tools = r.makeOptionsTools(chatOptions.tools)

	return opt
}

func (r *requestHelper) makeSystemMessage(msg *messages.SystemMessage) openai.ChatCompletionMessageParamUnion {
	return openai.SystemMessage(msg.Text())
}

func (r *requestHelper) makeUserMessage(msg *messages.UserMessage) openai.ChatCompletionMessageParamUnion {
	return openai.UserMessage(msg.Text())
}

func (r *requestHelper) makeAssistantMessage(msg *messages.AssistantMessage) openai.ChatCompletionMessageParamUnion {
	message := openai.AssistantMessage(msg.Text())
	for _, toolCall := range msg.ToolCalls() {
		message.OfAssistant.ToolCalls = append(
			message.OfAssistant.ToolCalls, openai.ChatCompletionMessageToolCallParam{
				ID: toolCall.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
				},
			})
	}
	return message
}

func (r *requestHelper) makeToolMessages(msg *messages.ToolResponseMessage) []openai.ChatCompletionMessageParamUnion {
	msgs := make([]openai.ChatCompletionMessageParamUnion, 0, len(msg.ToolResponses()))
	for _, toolResponse := range msg.ToolResponses() {
		msgs = append(msgs, openai.ToolMessage(toolResponse.ResponseData, toolResponse.ID))
	}
	return msgs
}

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

func (r *requestHelper) makeRequest(req *chat.Request) *openai.ChatCompletionNewParams {
	options := r.makeOptions(req.Options())
	msgs := r.makeMessages(req.Instructions())
	options.Messages = msgs
	return options
}

type responseHelper struct{}

func (r *responseHelper) makeResultAssistantMessage(message *openai.ChatCompletionMessage) *messages.AssistantMessage {
	toolCalls := make([]*messages.ToolCall, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		toolCalls = append(toolCalls, &messages.ToolCall{
			ID:        toolCall.ID,
			Type:      "function",
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		})
	}
	return messages.NewAssistantMessage(message.Content, nil, toolCalls)
}

func (r *responseHelper) makeResultMetadata(choice *openai.ChatCompletionChoice) *chat.ResultMetadata {
	if choice.FinishReason == "function_call" {
		choice.FinishReason = "tool_calls"
	}
	return &chat.ResultMetadata{
		FinishReason: chat.FinishReason(choice.FinishReason),
	}
}

func (r *responseHelper) makeResult(choice *openai.ChatCompletionChoice) (*chat.Result, error) {
	assistantMessage := r.makeResultAssistantMessage(&choice.Message)
	metadata := r.makeResultMetadata(choice)
	return chat.NewResult(assistantMessage, metadata)
}

func (r *responseHelper) makeResults(choices []openai.ChatCompletionChoice) ([]*chat.Result, error) {
	results := make([]*chat.Result, 0, len(choices))
	for _, choice := range choices {
		result, err := r.makeResult(&choice)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (r *responseHelper) makeMetadata(resp *openai.ChatCompletion) *chat.ResponseMetadata {
	metadata := &chat.ResponseMetadata{
		ID:    resp.ID,
		Model: resp.Model,
		Usage: &chat.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			OriginalUsage:    resp.Usage,
		},
	}
	return metadata
}

func (r *responseHelper) makeResponse(resp *openai.ChatCompletion) (*chat.Response, error) {
	results, err := r.makeResults(resp.Choices)
	if err != nil {
		return nil, err
	}
	metadata := r.makeMetadata(resp)
	return chat.NewResponse(results, metadata)
}
