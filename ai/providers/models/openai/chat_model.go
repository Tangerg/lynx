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

// buildToolSupport make chat.ToolSupport and inject tool params
func (c *ChatModel) buildToolSupport(chatRequest *chat.Request) *chat.ToolSupport {
	toolSupport := chat.NewToolSupport()
	toolSupport.RegisterTools(c.defaultOptions.tools...)

	requestOptions := chatRequest.Options
	if requestOptions == nil {
		return toolSupport
	}

	toolOptions, ok := requestOptions.(chat.ToolOptions)
	if !ok {
		return toolSupport
	}

	// use custom tools to override default tools
	toolSupport.RegisterTools(toolOptions.Tools()...)

	// merge tool parameters
	mergedToolParams := make(map[string]any)
	maps.Copy(mergedToolParams, c.defaultOptions.toolParams)
	// use custom tool parameters to override default tool parameters
	maps.Copy(mergedToolParams, toolOptions.ToolParams())

	toolOptions.SetToolParams(mergedToolParams)

	return toolSupport
}

func (c *ChatModel) call(ctx context.Context, chatRequest *chat.Request, toolSupport *chat.ToolSupport) (*chat.Response, error) {
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

	shouldInvokeTools, err := toolSupport.ShouldInvokeToolCalls(chatResponse)
	if err != nil {
		return nil, err
	}
	if !shouldInvokeTools {
		return chatResponse, nil
	}

	toolInvokeResult, err := toolSupport.InvokeToolCalls(ctx, chatRequest, chatResponse)
	if err != nil {
		return nil, err
	}

	if toolInvokeResult.ShouldResponse() {
		return toolInvokeResult.MakeResponse()
	}

	nextChatRequest, err := toolInvokeResult.MakeRequest()
	if err != nil {
		return nil, err
	}

	return c.call(ctx, nextChatRequest, toolSupport)
}

func (c *ChatModel) stream(ctx context.Context, chatRequest *chat.Request, toolSupport *chat.ToolSupport, yield func(*chat.Response, error) bool) {
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

	shouldInvokeTools, err := toolSupport.ShouldInvokeToolCalls(finalChatResponse)
	if err != nil {
		yield(nil, err)
		return
	}
	if !shouldInvokeTools {
		return
	}

	toolInvokeResult, err := toolSupport.InvokeToolCalls(ctx, chatRequest, finalChatResponse)
	if err != nil {
		yield(nil, err)
		return
	}

	if toolInvokeResult.ShouldResponse() {
		yield(toolInvokeResult.MakeResponse())
		return
	}

	nextChatRequest, err := toolInvokeResult.MakeRequest()
	if err != nil {
		yield(nil, err)
		return
	}

	_ = apiStreamResponse.Close()
	c.stream(ctx, nextChatRequest, toolSupport, yield)
}

func (c *ChatModel) Call(ctx context.Context, chatRequest *chat.Request) (*chat.Response, error) {
	toolSupport := c.buildToolSupport(chatRequest)

	if toolSupport.ShouldReturnDirect(chatRequest.Messages) {
		return toolSupport.MakeReturnDirectResponse(chatRequest.Messages)
	}

	return c.call(ctx, chatRequest, toolSupport)
}

func (c *ChatModel) Stream(ctx context.Context, chatRequest *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		toolSupport := c.buildToolSupport(chatRequest)

		if toolSupport.ShouldReturnDirect(chatRequest.Messages) {
			yield(toolSupport.MakeReturnDirectResponse(chatRequest.Messages))
			return
		}

		c.stream(ctx, chatRequest, toolSupport, yield)
	}
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return c.defaultOptions
}
