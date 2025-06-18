package openaiv2

import (
	"context"
	"errors"
	"io"

	"github.com/openai/openai-go"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/model"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/pkg/result"
	"github.com/Tangerg/lynx/pkg/safe"
	"github.com/Tangerg/lynx/pkg/stream"
)

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *Api
	defaultOptions *ChatOptions
}

func NewChatModel(apiKey model.ApiKey, defaultOptions *ChatOptions) (*ChatModel, error) {
	if defaultOptions == nil {
		return nil, errors.New("options is required")
	}
	return &ChatModel{
		api:            NewApi(apiKey),
		defaultOptions: defaultOptions,
	}, nil
}

func (c *ChatModel) makeToolHelper(req *chat.Request) *tool.Helper {
	helper := tool.NewHelper()

	helper.RegisterTools(c.defaultOptions.Tools()...)

	options := req.Options()
	if options != nil {
		toolOptions, ok := options.(tool.Options)
		if ok {
			helper.RegisterTools(toolOptions.Tools()...)
			toolOptions.SetToolParams(c.defaultOptions.ToolParams())
		}
	}
	return helper
}

func (c *ChatModel) makeApiChatCompletionRequest(req *chat.Request) *openai.ChatCompletionNewParams {
	return &openai.ChatCompletionNewParams{}
}

func (c *ChatModel) makeChatResponse(resp *openai.ChatCompletion) *chat.Response {
	return &chat.Response{}
}

func (c *ChatModel) call(ctx context.Context, req *chat.Request, helper *tool.Helper) (*chat.Response, error) {
	apiReq := c.makeApiChatCompletionRequest(req)
	apiResp, err := c.api.ChatCompletion(ctx, apiReq)
	if err != nil {
		return nil, err
	}
	resp := c.makeChatResponse(apiResp)

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

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	helper := c.makeToolHelper(req)

	chatResponse, err := helper.MakeReturnDirectChatResponse(req.Instructions())
	if err == nil {
		return chatResponse, nil
	}

	return c.call(ctx, req, helper)
}

func (c *ChatModel) stream(ctx context.Context, req *chat.Request, helper *tool.Helper, writer stream.Writer[result.Result[*chat.Response]]) {
	apiReq := c.makeApiChatCompletionRequest(req)
	apiStreamResp := c.api.ChatCompletionStream(ctx, apiReq)
	defer apiStreamResp.Close()

	fullAcc := openai.ChatCompletionAccumulator{}

	for apiStreamResp.Next() {
		chunk := apiStreamResp.Current()
		onceAcc := openai.ChatCompletionAccumulator{}
		onceAcc.AddChunk(chunk)
		resp := c.makeChatResponse(&onceAcc.ChatCompletion)
		err := writer.Write(ctx, result.Value(resp))
		if err != nil {
			return
		}
		fullAcc.AddChunk(chunk)
	}

	err := apiStreamResp.Err()
	if err != nil {
		_ = writer.Write(ctx, result.Error[*chat.Response](err))
		return
	}

	resp := c.makeChatResponse(&fullAcc.ChatCompletion)

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

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) (stream.Reader[result.Result[*chat.Response]], error) {
	// at least 1 to store mock chat response
	reader, writer := stream.Pipe[result.Result[*chat.Response]](1)
	defer func() {
		closer, ok := writer.(io.Closer)
		if ok {
			_ = closer.Close()
		}
	}()

	helper := c.makeToolHelper(req)

	chatResponse, err := helper.MakeReturnDirectChatResponse(req.Instructions())
	if err == nil {
		return reader, writer.Write(ctx, result.Value(chatResponse))
	}

	safe.Go(func() {
		c.stream(ctx, req, helper, writer)
	})
	return reader, nil
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return c.defaultOptions
}
