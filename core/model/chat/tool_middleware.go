package chat

import (
	"context"
	"iter"
)

// ToolMiddleware turns the model handler into a self-driving tool-calling
// loop. When the LLM emits tool calls the middleware executes them via
// the [ToolSupport] machinery and re-prompts the model with the results,
// repeating until the model produces a regular reply or every tool is
// configured for direct return.
//
// Use it via [NewToolMiddleware], which returns both call and stream
// halves so a single registration covers both paths.
//
// Example:
//
//	callMW, streamMW := chat.NewToolMiddleware()
//	resp, err := client.Chat().
//	    WithMiddlewares(callMW, streamMW).
//	    WithTools(myTool).
//	    Call().Response(ctx)
type ToolMiddleware struct{}

// NewToolMiddleware constructs the tool-calling middleware pair.
func NewToolMiddleware() (CallMiddleware, StreamMiddleware) {
	mw := &ToolMiddleware{}
	return mw.wrapCallHandler, mw.wrapStreamHandler
}

// wrapCallHandler is the call-side adapter — turns the middleware body
// into a [CallHandler] decorator.
func (m *ToolMiddleware) wrapCallHandler(next CallHandler) CallHandler {
	return CallHandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler is the stream-side adapter.
func (m *ToolMiddleware) wrapStreamHandler(next StreamHandler) StreamHandler {
	return StreamHandlerFunc(func(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
		return m.executeStream(ctx, req, next)
	})
}

// executeCall is the synchronous entry point: short-circuit when prior
// messages already indicate a direct return; otherwise enter the
// recursive call/tool loop.
func (m *ToolMiddleware) executeCall(ctx context.Context, req *Request, next CallHandler) (*Response, error) {
	support := NewToolSupport(len(req.Options.Tools))

	if support.ShouldReturnDirect(req.Messages) {
		return support.BuildReturnDirectResponse(req.Messages)
	}

	support.Register(req.Options.Tools...)
	return m.executeCallRecursively(ctx, req, next, support)
}

// executeCallRecursively runs one round of model + tool execution. If
// the model asks for tools and the tools want LLM follow-up, the
// function re-prompts and recurses.
func (m *ToolMiddleware) executeCallRecursively(ctx context.Context, req *Request, next CallHandler, support *ToolSupport) (*Response, error) {
	resp, err := next.Call(ctx, req)
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

	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	if result.ShouldReturn() {
		return result.BuildReturnResponse()
	}

	nextReq, err := result.BuildContinueRequest()
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, support)
}

// executeStream is the streaming entry point. Same shape as executeCall
// but delivers chunks through the iterator while accumulating them so
// the tool-calling loop can inspect a complete response when the stream
// closes.
func (m *ToolMiddleware) executeStream(ctx context.Context, req *Request, next StreamHandler) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		support := NewToolSupport(len(req.Options.Tools))

		if support.ShouldReturnDirect(req.Messages) {
			yield(support.BuildReturnDirectResponse(req.Messages))
			return
		}

		support.Register(req.Options.Tools...)
		m.executeStreamRecursively(ctx, req, next, support, yield)
	}
}

// executeStreamRecursively runs one streaming round: forward chunks to
// the caller while accumulating them, then inspect the accumulated
// response to decide whether to dispatch tool calls and re-stream.
func (m *ToolMiddleware) executeStreamRecursively(ctx context.Context, req *Request, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool) {
	accumulator := NewResponseAccumulator()

	for chunk, err := range next.Stream(ctx, req) {
		if err != nil {
			yield(chunk, err)
			return
		}

		accumulator.AddChunk(chunk)

		if !yield(chunk, nil) {
			return
		}
	}

	resp := &accumulator.Response
	shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
	if err != nil {
		yield(nil, err)
		return
	}
	if !shouldInvoke {
		return
	}

	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		yield(nil, err)
		return
	}

	if result.ShouldReturn() {
		yield(result.BuildReturnResponse())
		return
	}

	nextReq, err := result.BuildContinueRequest()
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, support, yield)
}
