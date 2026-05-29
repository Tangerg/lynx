package chat

import (
	"context"
	"fmt"
	"iter"
)

// DefaultMaxToolIterations bounds the self-driving tool loop. A model that
// keeps requesting tools — or a buggy tool whose result always re-triggers
// a call — would otherwise spin forever; the cap turns that into a
// [MaxToolIterationsError] instead. Mirrors embabel's
// MaxIterationsExceededException.
const DefaultMaxToolIterations = 50

// ToolLoopConfig tunes [NewToolMiddleware]. Every field is optional; the
// zero value yields the default loop (cap = [DefaultMaxToolIterations]).
type ToolLoopConfig struct {
	// MaxIterations caps the number of model calls the tool loop makes.
	// <= 0 falls back to [DefaultMaxToolIterations].
	MaxIterations int
}

// MaxToolIterationsError is returned when the tool-calling loop exceeds its
// configured iteration cap. Callers can detect it with [errors.As].
type MaxToolIterationsError struct {
	Limit int
}

func (e *MaxToolIterationsError) Error() string {
	return fmt.Sprintf("chat: tool loop exceeded %d iterations without a final reply", e.Limit)
}

// ToolMiddleware turns the model handler into a self-driving tool-calling
// loop. When the LLM emits tool calls the middleware executes them via
// the [ToolSupport] machinery and re-prompts the model with the results,
// repeating until the model produces a regular reply, every tool is
// configured for direct return, or the iteration cap is hit.
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
type ToolMiddleware struct {
	maxIterations int
}

// NewToolMiddleware constructs the tool-calling middleware pair. Pass an
// optional [ToolLoopConfig] to tune the loop; omit it for defaults.
func NewToolMiddleware(config ...ToolLoopConfig) (CallMiddleware, StreamMiddleware) {
	var cfg ToolLoopConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxToolIterations
	}

	mw := &ToolMiddleware{maxIterations: maxIterations}
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
	support := NewToolSupport(len(req.Tools))

	if support.ShouldReturnDirect(req.Messages) {
		return support.BuildReturnDirectResponse(req.Messages)
	}

	support.Register(req.Tools...)
	return m.executeCallRecursively(ctx, req, next, support, 1)
}

// executeCallRecursively runs one round of model + tool execution. If
// the model asks for tools and the tools want LLM follow-up, the
// function re-prompts and recurses. iteration is the 1-based model-call
// count; exceeding maxIterations aborts with a [MaxToolIterationsError].
func (m *ToolMiddleware) executeCallRecursively(ctx context.Context, req *Request, next CallHandler, support *ToolSupport, iteration int) (*Response, error) {
	if iteration > m.maxIterations {
		return nil, &MaxToolIterationsError{Limit: m.maxIterations}
	}

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
	return m.executeCallRecursively(ctx, nextReq, next, support, iteration+1)
}

// executeStream is the streaming entry point. Same shape as executeCall
// but delivers chunks through the iterator while accumulating them so
// the tool-calling loop can inspect a complete response when the stream
// closes.
func (m *ToolMiddleware) executeStream(ctx context.Context, req *Request, next StreamHandler) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		support := NewToolSupport(len(req.Tools))

		if support.ShouldReturnDirect(req.Messages) {
			yield(support.BuildReturnDirectResponse(req.Messages))
			return
		}

		support.Register(req.Tools...)
		m.executeStreamRecursively(ctx, req, next, support, yield, 1)
	}
}

// executeStreamRecursively runs one streaming round: forward chunks to
// the caller while accumulating them, then inspect the accumulated
// response to decide whether to dispatch tool calls and re-stream.
//
// Between turns the middleware emits a synthetic Response carrying the
// runtime-injected ToolMessage so external consumers see the same
// "assistant delta → tool result → assistant delta" timeline as in
// the request history. This is the discriminator established in §8.4
// of MESSAGE_PARTS_DESIGN: each yielded Response has exactly one of
// Result.AssistantMessage or Result.ToolMessage populated.
func (m *ToolMiddleware) executeStreamRecursively(ctx context.Context, req *Request, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool, iteration int) {
	if iteration > m.maxIterations {
		yield(nil, &MaxToolIterationsError{Limit: m.maxIterations})
		return
	}

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

	// Tool round-trip is happening in the middle of the loop. Surface
	// the ToolMessage to the stream consumer so the on-the-wire
	// timeline matches the message history we will hand the next
	// model turn.
	if result.toolMessage != nil {
		toolResp, err := newToolMessageResponse(result.toolMessage)
		if err == nil && !yield(toolResp, nil) {
			return
		}
	}

	nextReq, err := result.BuildContinueRequest()
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, support, yield, iteration+1)
}

// newToolMessageResponse wraps a [*ToolMessage] in a [*Response] whose
// Result.ToolMessage is set and Result.AssistantMessage is nil — the
// discriminator that distinguishes tool-injection deltas from model
// output deltas on the stream.
func newToolMessageResponse(tm *ToolMessage) (*Response, error) {
	result := &Result{
		ToolMessage: tm,
		Metadata:    &ResultMetadata{FinishReason: FinishReasonStop},
	}
	return NewResponse(result, &ResponseMetadata{})
}
