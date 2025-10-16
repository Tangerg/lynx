package openai

import (
	"context"
	"errors"
	"iter"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/ai/model"
	"github.com/Tangerg/lynx/ai/model/chat"
)

const (
	ChatCompletionOptions = "openai.chat.completion.options"
)

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	chatHelper
	api            *Api
	defaultOptions *chat.Options
}

func NewChatModel(apiKey model.ApiKey, defaultOptions *chat.Options, opts ...option.RequestOption) (*ChatModel, error) {
	if defaultOptions == nil {
		return nil, errors.New("default options cannot be nil")
	}

	api, err := NewApi(apiKey, opts...)
	if err != nil {
		return nil, err
	}

	return &ChatModel{
		chatHelper:     newChatHelper(defaultOptions),
		api:            api,
		defaultOptions: defaultOptions,
	}, nil
}

func (c *ChatModel) buildToolSupport(req *chat.Request) *chat.ToolSupport {
	support := chat.NewToolSupport()
	support.RegisterTools(c.defaultOptions.Tools...)

	// use custom tools to override default tools
	if req.Options != nil {
		support.RegisterTools(req.Options.Tools...)
	}

	return support
}

func (c *ChatModel) call(ctx context.Context, req *chat.Request, support *chat.ToolSupport) (*chat.Response, error) {
	apiReq, err := c.buildChatRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	resp, err := c.buildChatResponse(apiReq, apiResp)
	if err != nil {
		return nil, err
	}

	shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !shouldInvoke {
		return resp, nil
	}

	invokeResult, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	if invokeResult.ShouldReturn() {
		return invokeResult.BuildReturnResponse()
	}

	nextReq, err := invokeResult.BuildContinueRequest()
	if err != nil {
		return nil, err
	}

	return c.call(ctx, nextReq, support)
}

func (c *ChatModel) stream(ctx context.Context, req *chat.Request, support *chat.ToolSupport, yield func(*chat.Response, error) bool) {
	apiReq, err := c.buildChatRequest(req)
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

	var (
		fullAcc = openai.ChatCompletionAccumulator{}
		resp    *chat.Response
	)

	for apiStream.Next() {
		chunk := apiStream.Current()
		chunkAcc := openai.ChatCompletionAccumulator{}
		chunkAcc.AddChunk(chunk)

		resp, err = c.buildChatResponse(apiReq, &chunkAcc.ChatCompletion)
		if err != nil {
			yield(nil, err)
			return
		}

		if !yield(resp, nil) {
			return
		}

		fullAcc.AddChunk(chunk)
	}

	if err = apiStream.Err(); err != nil {
		yield(nil, err)
		return
	}

	finalResp, err := c.buildChatResponse(apiReq, &fullAcc.ChatCompletion)
	if err != nil {
		yield(nil, err)
		return
	}

	shouldInvoke, err := support.ShouldInvokeToolCalls(finalResp)
	if err != nil {
		yield(nil, err)
		return
	}
	if !shouldInvoke {
		return
	}

	invokeResult, err := support.InvokeToolCalls(ctx, req, finalResp)
	if err != nil {
		yield(nil, err)
		return
	}

	if invokeResult.ShouldReturn() {
		yield(invokeResult.BuildReturnResponse())
		return
	}

	nextReq, err := invokeResult.BuildContinueRequest()
	if err != nil {
		yield(nil, err)
		return
	}

	_ = apiStream.Close()
	c.stream(ctx, nextReq, support, yield)
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	support := c.buildToolSupport(req)

	if support.ShouldReturnDirect(req.Messages) {
		return support.BuildReturnDirectResponse(req.Messages)
	}

	return c.call(ctx, req, support)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		support := c.buildToolSupport(req)

		if support.ShouldReturnDirect(req.Messages) {
			yield(support.BuildReturnDirectResponse(req.Messages))
			return
		}

		c.stream(ctx, req, support, yield)
	}
}

func (c *ChatModel) DefaultOptions() *chat.Options {
	return c.defaultOptions
}

func (c *ChatModel) Info() chat.ModelInfo {
	return chat.ModelInfo{
		Provider: "OpenAI",
	}
}
