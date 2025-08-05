package openai

import (
	"context"
	"errors"
	"iter"
	"maps"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/tool"
)

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	chatHelper
	api            *Api
	defaultOptions *ChatOptions
}

func NewChatModel(apiKey model.ApiKey, defaultOptions *ChatOptions, opts ...option.RequestOption) (*ChatModel, error) {
	if defaultOptions == nil {
		return nil, errors.New("defaultOptions is required")
	}

	apiClient, err := NewApi(apiKey, opts...)
	if err != nil {
		return nil, err
	}

	return &ChatModel{
		chatHelper:     newChatHelper(defaultOptions),
		api:            apiClient,
		defaultOptions: defaultOptions,
	}, nil
}

// buildToolHelper make tool.Helper and inject tool params
func (c *ChatModel) buildToolHelper(chatRequest *chat.Request) *tool.Helper {
	toolHelper := tool.NewHelper()
	toolHelper.RegisterTools(c.defaultOptions.tools...)

	requestOptions := chatRequest.Options()
	if requestOptions == nil {
		return toolHelper
	}

	toolOptions, ok := requestOptions.(tool.Options)
	if !ok {
		return toolHelper
	}

	// use custom tools to override default tools
	toolHelper.RegisterTools(toolOptions.Tools()...)

	// merge tool parameters
	mergedToolParams := make(map[string]any)
	maps.Copy(mergedToolParams, c.defaultOptions.toolParams)
	// use custom tool parameters to override default tool parameters
	maps.Copy(mergedToolParams, toolOptions.ToolParams())

	toolOptions.SetToolParams(mergedToolParams)

	return toolHelper
}

func (c *ChatModel) call(ctx context.Context, chatRequest *chat.Request, toolHelper *tool.Helper) (*chat.Response, error) {
	apiRequest, err := c.makeApiChatCompletionRequest(chatRequest)
	if err != nil {
		return nil, err
	}

	apiResponse, err := c.api.ChatCompletion(ctx, apiRequest)
	if err != nil {
		return nil, err
	}

	chatResponse, err := c.makeChatResponse(apiRequest, apiResponse)
	if err != nil {
		return nil, err
	}

	shouldInvokeTools, err := toolHelper.ShouldInvokeToolCalls(chatResponse)
	if err != nil {
		return nil, err
	}
	if !shouldInvokeTools {
		return chatResponse, nil
	}

	toolInvokeResult, err := toolHelper.InvokeToolCalls(ctx, chatRequest, chatResponse)
	if err != nil {
		return nil, err
	}

	if toolInvokeResult.ShouldMakeChatResponse() {
		return toolInvokeResult.MakeChatResponse()
	}

	nextChatRequest, err := toolInvokeResult.MakeChatRequest()
	if err != nil {
		return nil, err
	}

	return c.call(ctx, nextChatRequest, toolHelper)
}

func (c *ChatModel) stream(ctx context.Context, chatRequest *chat.Request, toolHelper *tool.Helper, yield func(*chat.Response, error) bool) {
	apiRequest, err := c.makeApiChatCompletionRequest(chatRequest)
	if err != nil {
		yield(nil, err)
		return
	}

	apiStreamResponse, err := c.api.ChatCompletionStream(ctx, apiRequest)
	if err != nil {
		yield(nil, err)
		return
	}
	defer apiStreamResponse.Close()

	var (
		fullAccumulator = openai.ChatCompletionAccumulator{}
		chatResponse    *chat.Response
	)

	for apiStreamResponse.Next() {
		chunk := apiStreamResponse.Current()
		chunkAccumulator := openai.ChatCompletionAccumulator{}
		chunkAccumulator.AddChunk(chunk)

		chatResponse, err = c.makeChatResponse(apiRequest, &chunkAccumulator.ChatCompletion)
		if err != nil {
			yield(nil, err)
			return
		}

		if !yield(chatResponse, nil) {
			return
		}

		fullAccumulator.AddChunk(chunk)
	}

	if err = apiStreamResponse.Err(); err != nil {
		yield(nil, err)
		return
	}

	finalChatResponse, err := c.makeChatResponse(apiRequest, &fullAccumulator.ChatCompletion)
	if err != nil {
		yield(nil, err)
		return
	}

	shouldInvokeTools, err := toolHelper.ShouldInvokeToolCalls(finalChatResponse)
	if err != nil {
		yield(nil, err)
		return
	}
	if !shouldInvokeTools {
		return
	}

	toolInvokeResult, err := toolHelper.InvokeToolCalls(ctx, chatRequest, finalChatResponse)
	if err != nil {
		yield(nil, err)
		return
	}

	if toolInvokeResult.ShouldMakeChatResponse() {
		yield(toolInvokeResult.MakeChatResponse())
		return
	}

	nextChatRequest, err := toolInvokeResult.MakeChatRequest()
	if err != nil {
		yield(nil, err)
		return
	}

	_ = apiStreamResponse.Close()
	c.stream(ctx, nextChatRequest, toolHelper, yield)
}

func (c *ChatModel) Call(ctx context.Context, chatRequest *chat.Request) (*chat.Response, error) {
	toolHelper := c.buildToolHelper(chatRequest)

	if toolHelper.ShouldReturnDirect(chatRequest.Instructions()) {
		return toolHelper.MakeReturnDirectChatResponse(chatRequest.Instructions())
	}

	return c.call(ctx, chatRequest, toolHelper)
}

func (c *ChatModel) Stream(ctx context.Context, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		toolHelper := c.buildToolHelper(chatRequest)

		if toolHelper.ShouldReturnDirect(chatRequest.Instructions()) {
			yield(toolHelper.MakeReturnDirectChatResponse(chatRequest.Instructions()))
			return
		}

		c.stream(ctx, chatRequest, toolHelper, yield)
	}
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return c.defaultOptions
}
