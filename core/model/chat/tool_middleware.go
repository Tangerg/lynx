package chat

import (
	"context"
	"iter"
)

// ToolMiddleware handles tool execution in the middleware layer.
// It intercepts chat responses containing tool calls, executes the tools,
// and recursively calls the model with tool results until a final response is obtained.
type ToolMiddleware struct{}

// NewToolMiddleware creates a new tool execution middleware.
// Returns both call and stream middleware functions for use in the middleware chain.
func NewToolMiddleware() (CallMiddleware, StreamMiddleware) {
	mw := &ToolMiddleware{}
	return mw.wrapCallHandler, mw.wrapStreamHandler
}

// executeCallRecursively processes chat requests with tool execution support.
// It recursively calls the model when tool invocations are required,
// building a complete conversation flow with tool results.
func (m *ToolMiddleware) executeCallRecursively(ctx context.Context, req *Request, next CallHandler, support *ToolSupport) (*Response, error) {
	// Call the next handler (eventually reaching the model)
	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}

	// Check if the response contains tool calls that need execution
	shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}

	if !shouldInvoke {
		return resp, nil
	}

	// Execute the tool calls
	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	// Check if tool result should be returned directly without further LLM interaction
	if result.ShouldReturn() {
		return result.BuildReturnResponse()
	}

	// Build a new request with tool results and continue the conversation
	continueReq, err := result.BuildContinueRequest()
	if err != nil {
		return nil, err
	}

	// Recursively call with the updated request
	return m.executeCallRecursively(ctx, continueReq, next, support)
}

// executeCall is the main entry point for synchronous call handling with tool support.
// It sets up tool support, checks for direct tool returns, and initiates the recursive call chain.
func (m *ToolMiddleware) executeCall(ctx context.Context, req *Request, next CallHandler) (*Response, error) {
	support := NewToolSupport(len(req.Options.Tools))

	// Check if any existing messages indicate a direct tool return
	if support.ShouldReturnDirect(req.Messages) {
		return support.BuildReturnDirectResponse(req.Messages)
	}

	// Register available tools
	support.RegisterTools(req.Options.Tools...)

	// Start recursive processing
	return m.executeCallRecursively(ctx, req, next, support)
}

// executeStreamRecursively processes streaming chat requests with tool execution support.
// It accumulates streaming chunks, executes tools when needed, and recursively streams
// the conversation with tool results.
func (m *ToolMiddleware) executeStreamRecursively(ctx context.Context, req *Request, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool) {
	// Accumulate streaming chunks into a complete response
	accumulator := NewResponseAccumulator()

	for chunk, err := range next.Stream(ctx, req) {
		if err != nil {
			yield(chunk, err)
			return
		}

		accumulator.AddChunk(chunk)

		// Yield each chunk to the caller for real-time processing
		if !yield(chunk, nil) {
			return
		}
	}

	// Check if the accumulated response contains tool calls
	resp := &accumulator.Response
	shouldInvoke, err := support.ShouldInvokeToolCalls(resp)
	if err != nil {
		yield(nil, err)
		return
	}

	if !shouldInvoke {
		return
	}

	// Execute the tool calls
	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		yield(nil, err)
		return
	}

	// Check if tool result should be returned directly
	if result.ShouldReturn() {
		yield(result.BuildReturnResponse())
		return
	}

	// Build a new request with tool results and continue streaming
	continueReq, err := result.BuildContinueRequest()
	if err != nil {
		yield(nil, err)
		return
	}

	// Recursively stream with the updated request
	m.executeStreamRecursively(ctx, continueReq, next, support, yield)
}

// executeStream is the main entry point for streaming handling with tool support.
// It sets up tool support, checks for direct tool returns, and initiates the recursive stream chain.
func (m *ToolMiddleware) executeStream(ctx context.Context, req *Request, next StreamHandler) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		support := NewToolSupport(len(req.Options.Tools))

		// Check if any existing messages indicate a direct tool return
		if support.ShouldReturnDirect(req.Messages) {
			yield(support.BuildReturnDirectResponse(req.Messages))
			return
		}

		// Register available tools
		support.RegisterTools(req.Options.Tools...)

		// Start recursive streaming
		m.executeStreamRecursively(ctx, req, next, support, yield)
	}
}

// wrapCallHandler wraps the call handler with tool execution middleware.
func (m *ToolMiddleware) wrapCallHandler(next CallHandler) CallHandler {
	return CallHandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler wraps the stream handler with tool execution middleware.
func (m *ToolMiddleware) wrapStreamHandler(next StreamHandler) StreamHandler {
	return StreamHandlerFunc(func(ctx context.Context, req *Request) iter.Seq2[*Response, error] {
		return m.executeStream(ctx, req, next)
	})
}
