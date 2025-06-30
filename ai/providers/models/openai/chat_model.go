package openai

import (
	"context"
	"errors"
	"maps"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/model"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/safe"
	"github.com/Tangerg/lynx/pkg/stream"
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

func (c *ChatModel) call(ctx context.Context, req *chat.Request, helper *tool.Helper) (*chat.Response, error) {
	apiReq, err := c.makeApiChatCompletionRequest(req)
	if err != nil {
		return nil, err
	}

	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	resp, err := c.makeChatResponse(apiReq, apiResp)
	if err != nil {
		return nil, err
	}

	shouldInvoke, err := helper.ShouldInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !shouldInvoke {
		return resp, nil
	}

	invokeResult, err := helper.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	if invokeResult.ShouldMakeChatResponse() {
		return invokeResult.MakeChatResponse()
	}

	nextReq, err := invokeResult.MakeChatRequest()
	if err != nil {
		return nil, err
	}

	return c.call(ctx, nextReq, helper)
}

func (c *ChatModel) stream(ctx context.Context, req *chat.Request, helper *tool.Helper, writer stream.Writer[result.Result[*chat.Response]]) {
	apiReq, err := c.makeApiChatCompletionRequest(req)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	apiStreamResp, err := c.api.ChatCompletionStream(ctx, apiReq)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}
	defer apiStreamResp.Close()

	var (
		fullAcc = openai.ChatCompletionAccumulator{}
		resp    *chat.Response
	)

	for apiStreamResp.Next() {
		chunk := apiStreamResp.Current()
		onceAcc := openai.ChatCompletionAccumulator{}
		onceAcc.AddChunk(chunk)

		resp, err = c.makeChatResponse(apiReq, &onceAcc.ChatCompletion)
		if err != nil {
			_ = writer.Write(ctx, result.Error[*chat.Response](err))
			return
		}

		err = writer.Write(ctx, result.Value(resp))
		if err != nil {
			return
		}

		fullAcc.AddChunk(chunk)
	}

	err = apiStreamResp.Err()
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	resp, err = c.makeChatResponse(apiReq, &fullAcc.ChatCompletion)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	shouldInvoke, err := helper.ShouldInvokeToolCalls(resp)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}
	if !shouldInvoke {
		return
	}

	invokeResult, err := helper.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	if invokeResult.ShouldMakeChatResponse() {
		resp, err = invokeResult.MakeChatResponse()
		if err != nil {
			_ = writer.Write(ctx, result.Error[*chat.Response](err))
		} else {
			_ = writer.Write(ctx, result.Value(resp))
		}
		return
	}

	nextReq, err := invokeResult.MakeChatRequest()
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	_ = apiStreamResp.Close()
	c.stream(ctx, nextReq, helper, writer)
}

// beforeChat make tool.Helper and inject tool params
func (c *ChatModel) beforeChat(req *chat.Request) *tool.Helper {
	toolHelper := tool.NewHelper()

	toolHelper.RegisterTools(c.defaultOptions.tools...)

	options := req.Options()
	if options == nil {
		return toolHelper
	}

	toolOptions, ok := options.(tool.Options)
	if !ok {
		return toolHelper
	}

	// use custom tools to override default tools
	toolHelper.RegisterTools(toolOptions.Tools()...)

	toolParams := make(map[string]any)
	maps.Copy(toolParams, c.defaultOptions.toolParams)
	// use custom tool parameters to override default tool parameters
	maps.Copy(toolParams, toolOptions.ToolParams())

	toolOptions.SetToolParams(toolParams)

	return toolHelper
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	toolHelper := c.beforeChat(req)

	if toolHelper.ShouldReturnDirect(req.Instructions()) {
		return toolHelper.MakeReturnDirectChatResponse(req.Instructions())
	}

	return c.call(ctx, req, toolHelper)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) (stream.Reader[result.Result[*chat.Response]], error) {
	// at least 1 to store mock chat response
	streamer := stream.NewStream[result.Result[*chat.Response]](1)
	toolHelper := c.beforeChat(req)

	if toolHelper.ShouldReturnDirect(req.Instructions()) {
		chatResponse, err := toolHelper.MakeReturnDirectChatResponse(req.Instructions())
		if err != nil {
			return nil, err
		}

		_ = streamer.Close()
		return streamer, streamer.Write(ctx, result.Value(chatResponse))
	}

	safe.Go(func() {
		defer streamer.Close()
		c.stream(ctx, req, toolHelper, streamer)
	})

	return streamer, nil
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return c.defaultOptions
}
