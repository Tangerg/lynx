package openai

import (
	"context"
	"errors"

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
		return nil, errors.New("options is required")
	}
	return &ChatModel{
		chatHelper:     newChatHelper(defaultOptions),
		api:            NewApi(apiKey, opts...),
		defaultOptions: defaultOptions,
	}, nil
}

func (c *ChatModel) call(ctx context.Context, req *chat.Request, helper *tool.Helper) (*chat.Response, error) {
	apiReq := c.makeApiChatCompletionRequest(req)
	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}

	resp, err := c.makeChatResponse(apiResp)
	if err != nil {
		return nil, err
	}

	shouldInvokeToolCalls, err := helper.ShouldInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !shouldInvokeToolCalls {
		return resp, nil
	}
	invokeResult, err := helper.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}
	if invokeResult.ShouldMakeChatResponse() {
		return invokeResult.MakeChatResponse()
	}
	nexReq, err := invokeResult.MakeChatRequest()
	if err != nil {
		return nil, err
	}
	return c.call(ctx, nexReq, helper)
}

func (c *ChatModel) stream(ctx context.Context, req *chat.Request, helper *tool.Helper, writer stream.Writer[result.Result[*chat.Response]]) {
	apiReq := c.makeApiChatCompletionRequest(req)
	apiStreamResp, err := c.api.ChatCompletionStream(ctx, apiReq)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}
	defer apiStreamResp.Close()

	fullAcc := openai.ChatCompletionAccumulator{}

	for apiStreamResp.Next() {
		chunk := apiStreamResp.Current()
		onceAcc := openai.ChatCompletionAccumulator{}
		onceAcc.AddChunk(chunk)
		resp, err1 := c.makeChatResponse(&onceAcc.ChatCompletion)
		if err1 != nil {
			_ = writer.Write(ctx, result.Error[*chat.Response](err1))
			return
		}
		err1 = writer.Write(ctx, result.Value(resp))
		if err1 != nil {
			return
		}
		fullAcc.AddChunk(chunk)
	}

	err = apiStreamResp.Err()
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	resp, err := c.makeChatResponse(&fullAcc.ChatCompletion)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	shouldInvokeToolCalls, err := helper.ShouldInvokeToolCalls(resp)
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}
	if !shouldInvokeToolCalls {
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

	toolHelper.RegisterTools(c.defaultOptions.Tools()...)

	options := req.Options()
	if options != nil {
		toolOptions, ok := options.(tool.Options)
		if ok {
			toolHelper.RegisterTools(toolOptions.Tools()...)
			toolOptions.SetToolParams(c.defaultOptions.ToolParams())
		}
	}
	return toolHelper
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	toolHelper := c.beforeChat(req)

	chatResponse, err := toolHelper.MakeReturnDirectChatResponse(req.Instructions())
	if err == nil {
		return chatResponse, nil
	}

	return c.call(ctx, req, toolHelper)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) (stream.Reader[result.Result[*chat.Response]], error) {
	// at least 1 to store mock chat response
	streamer := stream.NewStream[result.Result[*chat.Response]](1)
	toolHelper := c.beforeChat(req)

	chatResponse, err := toolHelper.MakeReturnDirectChatResponse(req.Instructions())
	if err == nil {
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
