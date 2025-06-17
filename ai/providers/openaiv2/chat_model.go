package openaiv2

import (
	"context"
	"errors"
	"io"

	"github.com/openai/openai-go"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/tool"
	"github.com/Tangerg/lynx/ai/providers/openaiv2/api"
	"github.com/Tangerg/lynx/pkg/safe"
	"github.com/Tangerg/lynx/pkg/stream"
)

var _ chat.Model = (*ChatModel)(nil)

type ChatModel struct {
	api            *api.OpenAIApi
	defaultOptions *ChatOptions
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

func (c *ChatModel) makeChunkChatResponse(req *openai.ChatCompletionChunk) *chat.Response {
	return &chat.Response{}
}

func (c *ChatModel) makeErrorChatResponse(err error) *chat.Response {
	return &chat.Response{}
}

func (c *ChatModel) call(ctx context.Context, req *chat.Request, helper *tool.Helper) (*chat.Response, error) {
	if helper.ShouldReturnDirect(req.Instructions()) {
		//todo
	}

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
	if invokeResult.ShouldBuildChatResponse() {
		return invokeResult.BuildChatResponse()
	}
	nexReq, err := invokeResult.BuildChatRequest()
	if err != nil {
		return nil, err
	}
	return c.call(ctx, nexReq, helper)
}

func (c *ChatModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) {
	helper := c.makeToolHelper(req)
	return c.call(ctx, req, helper)
}

func (c *ChatModel) stream(ctx context.Context, req *chat.Request, helper *tool.Helper, writer stream.Writer[*chat.Response]) {
	defer func() {
		closer, ok := writer.(io.Closer)
		if ok {
			closer.Close()
		}
	}()

	if helper.ShouldReturnDirect(req.Instructions()) {
		//todo
	}

	apiReq := c.makeApiChatCompletionRequest(req)
	apiStreamResp := c.api.ChatCompletionStream(ctx, apiReq)
	defer apiStreamResp.Close()

	acc := openai.ChatCompletionAccumulator{}

	for apiStreamResp.Next() {
		chunk := apiStreamResp.Current()
		resp := c.makeChunkChatResponse(&chunk)
		err := writer.Write(ctx, resp)
		// if context error
		if !errors.Is(err, stream.ErrStreamClosed) {
			return
		}
		acc.AddChunk(chunk)
	}
	err := apiStreamResp.Err()
	if err != nil {
		return
	}

	resp := c.makeChatResponse(&acc.ChatCompletion)

	shouldInvokeToolCalls, err := helper.ShouldInvokeToolCalls(resp)
	if err != nil {
		resp = c.makeErrorChatResponse(err)
		_ = writer.Write(ctx, resp)
		return
	}
	if !shouldInvokeToolCalls {
		return
	}
	invokeResult, err := helper.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		resp = c.makeErrorChatResponse(err)
		_ = writer.Write(ctx, resp)
		return
	}
	if invokeResult.ShouldBuildChatResponse() {
		resp, err = invokeResult.BuildChatResponse()
		if err != nil {
			resp = c.makeErrorChatResponse(err)
			_ = writer.Write(ctx, resp)
		} else {
			_ = writer.Write(ctx, resp)
		}
	}
	nexReq, err := invokeResult.BuildChatRequest()
	if err != nil {
		resp = c.makeErrorChatResponse(err)
		_ = writer.Write(ctx, resp)
	}
	c.stream(ctx, nexReq, helper, writer)
}

func (c *ChatModel) Stream(ctx context.Context, req *chat.Request) (stream.Reader[*chat.Response], error) {
	reader, writer := stream.Pipe[*chat.Response]()
	helper := c.makeToolHelper(req)
	safe.Go(func() {
		c.stream(ctx, req, helper, writer)
	})
	return reader, nil
}

func (c *ChatModel) DefaultOptions() chat.Options {
	return c.defaultOptions
}
