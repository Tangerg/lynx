package openaiv2

import (
	"encoding/json"

	"github.com/openai/openai-go"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/tool"
)

type chatHelper struct {
	defaultOptions *ChatOptions
}

func newChatHelper(defaultOptions *ChatOptions) chatHelper {
	return chatHelper{
		defaultOptions: defaultOptions,
	}
}

// beforeChat make tool.Helper and inject tool params
func (h *chatHelper) beforeChat(req *chat.Request) *tool.Helper {
	toolHelper := tool.NewHelper()

	toolHelper.RegisterTools(h.defaultOptions.Tools()...)

	options := req.Options()
	if options != nil {
		toolOptions, ok := options.(tool.Options)
		if ok {
			toolHelper.RegisterTools(toolOptions.Tools()...)
			toolOptions.SetToolParams(h.defaultOptions.ToolParams())
		}
	}
	return toolHelper
}

func (h *chatHelper) makeApiChatCompletionRequestOptionsTools(tools []tool.Tool) []openai.ChatCompletionToolParam {
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

func (h *chatHelper) makeApiChatCompletionRequestOptions(options chat.Options) *openai.ChatCompletionNewParams {
	chatOptions, _ := MergeChatOptions(h.defaultOptions, options)

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
	opt.Tools = h.makeApiChatCompletionRequestOptionsTools(chatOptions.tools)

	return opt
}

func (h *chatHelper) makeApiChatCompletionRequestMessage(msg messages.Message) openai.ChatCompletionMessageParamUnion {
	if msg.Type().IsSystem() {
		return openai.SystemMessage(msg.Text())
	}
	if msg.Type().IsUser() {
		return openai.UserMessage(msg.Text())
	}
	if msg.Type().IsAssistant() {
		return openai.AssistantMessage(msg.Text())
	}
	//if msg.Type().IsTool(){
	//	return openai.ToolMessage()
	//}
	return openai.UserMessage(msg.Text())
}

func (h *chatHelper) makeApiChatCompletionRequestMessages(msgs []messages.Message) []openai.ChatCompletionMessageParamUnion {
	rv := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, msg := range msgs {
		rv = append(rv, h.makeApiChatCompletionRequestMessage(msg))
	}
	return rv
}

func (h *chatHelper) makeApiChatCompletionRequest(req *chat.Request) *openai.ChatCompletionNewParams {
	options := h.makeApiChatCompletionRequestOptions(req.Options())
	msgs := h.makeApiChatCompletionRequestMessages(req.Instructions())
	options.Messages = msgs
	return options
}

func (h *chatHelper) makeChatResponseResultAssistantMessage(message *openai.ChatCompletionMessage) *messages.AssistantMessage {
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

func (h *chatHelper) makeChatResponseResultMetadata(choice *openai.ChatCompletionChoice) *chat.ResultMetadata {
	if choice.FinishReason == "function_call" {
		choice.FinishReason = "tool_calls"
	}
	return &chat.ResultMetadata{
		FinishReason: chat.FinishReason(choice.FinishReason),
	}
}

func (h *chatHelper) makeChatResponseResult(choice *openai.ChatCompletionChoice) (*chat.Result, error) {
	assistantMessage := h.makeChatResponseResultAssistantMessage(&choice.Message)
	metadata := h.makeChatResponseResultMetadata(choice)
	return chat.NewResult(assistantMessage, metadata)
}

func (h *chatHelper) makeChatResponseResults(choices []openai.ChatCompletionChoice) ([]*chat.Result, error) {
	results := make([]*chat.Result, 0, len(choices))
	for _, choice := range choices {
		result, err := h.makeChatResponseResult(&choice)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (h *chatHelper) makeChatResponseMetadata(resp *openai.ChatCompletion) *chat.ResponseMetadata {
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

func (h *chatHelper) makeChatResponse(resp *openai.ChatCompletion) (*chat.Response, error) {
	results, err := h.makeChatResponseResults(resp.Choices)
	if err != nil {
		return nil, err
	}
	metadata := h.makeChatResponseMetadata(resp)
	return chat.NewResponse(results, metadata)
}

func MergeChatOptions(options *ChatOptions, opts ...chat.Options) (*ChatOptions, error) {
	fork := options.Fork()
	for _, o := range opts {
		if o == nil {
			continue
		}
		builder := fork.
			WithModel(o.Model()).
			WithFrequencyPenalty(o.FrequencyPenalty()).
			WithMaxTokens(o.MaxTokens()).
			WithPresencePenalty(o.PresencePenalty()).
			WithStopSequences(o.StopSequences()).
			WithTemperature(o.Temperature()).
			WithTopK(o.TopK()).
			WithTopP(o.TopP())
		toolOptions, ok := o.(tool.Options)
		if ok {
			builder.
				WithTools(toolOptions.Tools()).
				WithToolParams(toolOptions.ToolParams())
		}
	}
	return fork.Build()
}
