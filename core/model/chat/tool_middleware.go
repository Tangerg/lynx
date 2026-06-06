package chat

import (
	"context"
	"fmt"
	"iter"
	"strings"
)

// DefaultMaxToolIterations bounds the self-driving tool loop. A model that
// keeps requesting tools — or a buggy tool whose result always re-triggers
// a call — would otherwise spin forever; the cap turns that into a
// [MaxToolIterationsError] instead. Mirrors embabel's
// MaxIterationsExceededException.
const DefaultMaxToolIterations = 50

// emptyResponseNudge is the follow-up prompt sent when a model returns an
// empty reply and [ToolLoopConfig.FeedbackOnEmptyResponse] is enabled.
const emptyResponseNudge = "Your previous reply was empty. Please provide a complete answer, or call one of the available tools."

// ToolLoopConfig tunes [NewToolMiddleware]. Every field is optional; the
// zero value yields the default loop (cap = [DefaultMaxToolIterations], no
// recovery feedback).
type ToolLoopConfig struct {
	// MaxIterations caps the number of model calls the tool loop makes.
	// <= 0 falls back to [DefaultMaxToolIterations].
	MaxIterations int

	// FeedbackOnUnknownTool, when true, makes a call to an unregistered
	// tool produce an error result fed back to the model (so it can pick a
	// real tool) instead of aborting the request. Default off preserves the
	// strict behavior.
	FeedbackOnUnknownTool bool

	// FeedbackOnEmptyResponse, when true, answers an empty model reply (no
	// text and no tool calls) with a nudge and re-prompts the model once
	// instead of returning the empty reply. Default off.
	FeedbackOnEmptyResponse bool

	// FeedbackOnToolError, when true, turns a tool execution failure into an
	// error result fed back to the model (so it can adjust and continue)
	// instead of aborting the whole request. The sibling of
	// FeedbackOnUnknownTool for execution errors. Default off preserves the
	// strict behavior (a tool error aborts the loop).
	FeedbackOnToolError bool
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
	maxIterations   int
	feedbackUnknown bool
	feedbackEmpty   bool
	feedbackError   bool
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

	mw := &ToolMiddleware{
		maxIterations:   maxIterations,
		feedbackUnknown: cfg.FeedbackOnUnknownTool,
		feedbackEmpty:   cfg.FeedbackOnEmptyResponse,
		feedbackError:   cfg.FeedbackOnToolError,
	}
	return mw.wrapCallHandler, mw.wrapStreamHandler
}

// toolLoopState carries the per-loop bookkeeping threaded through the
// recursion: the 1-based model-call count and whether the one-shot
// empty-response nudge has already been spent.
type toolLoopState struct {
	iteration    int
	emptyRetried bool
}

func (s toolLoopState) next() toolLoopState {
	s.iteration++
	return s
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

// newToolSupport builds the per-loop support, applying the middleware's
// unknown-tool policy.
func (m *ToolMiddleware) newToolSupport(toolCount int) *ToolSupport {
	support := NewToolSupport(toolCount)
	support.SetFeedbackOnUnknownTool(m.feedbackUnknown)
	support.SetFeedbackOnToolError(m.feedbackError)
	return support
}

// executeCall is the synchronous entry point: short-circuit when prior
// messages already indicate a direct return; otherwise enter the
// recursive call/tool loop.
func (m *ToolMiddleware) executeCall(ctx context.Context, req *Request, next CallHandler) (*Response, error) {
	support := m.newToolSupport(len(req.Tools))

	if support.ShouldReturnDirect(req.Messages) {
		return support.BuildReturnDirectResponse(req.Messages)
	}

	support.Register(req.Tools...)

	// HITL resume: when the conversation tail is an assistant turn whose tool
	// calls aren't fully answered (a prior segment interrupted for human
	// input and its conversation was fed back), execute the still-pending
	// calls and continue — without re-invoking the model for completed work.
	if assistant, done, pending := trailingPendingToolCalls(req.Messages); assistant != nil {
		return m.resumeCallRound(ctx, req, assistant, done, pending, next, support, toolLoopState{iteration: priorModelRounds(req.Messages)})
	}

	return m.executeCallRecursively(ctx, req, next, support, toolLoopState{iteration: 1})
}

// executeCallRecursively runs one round of model + tool execution. If
// the model asks for tools and the tools want LLM follow-up, the
// function re-prompts and recurses. state.iteration is the 1-based
// model-call count; exceeding maxIterations aborts with a
// [MaxToolIterationsError].
func (m *ToolMiddleware) executeCallRecursively(ctx context.Context, req *Request, next CallHandler, support *ToolSupport, state toolLoopState) (*Response, error) {
	if state.iteration > m.maxIterations {
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
		if nudgeReq, ok, err := m.maybeNudgeEmpty(req, resp, state); err != nil {
			return nil, err
		} else if ok {
			st := state.next()
			st.emptyRetried = true
			return m.executeCallRecursively(ctx, nudgeReq, next, support, st)
		}
		return resp, nil
	}

	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		return nil, err
	}

	if result.interrupt != nil {
		// HITL: a tool call interrupted the round. Propagate a
		// *ToolLoopInterrupted carrying the resumable conversation (prior
		// turns + this round's assistant tool-call message + the results
		// already produced) so the caller can save it, park, and resume.
		return nil, m.wrapInterrupt(resp.Result.AssistantMessage, result.interrupt.done, result.interrupt.cause)
	}

	if result.ShouldReturn() {
		return result.BuildReturnResponse()
	}

	nextReq, err := result.BuildContinueRequest()
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, support, state.next())
}

// executeStream is the streaming entry point. Same shape as executeCall
// but delivers chunks through the iterator while accumulating them so
// the tool-calling loop can inspect a complete response when the stream
// closes.
func (m *ToolMiddleware) executeStream(ctx context.Context, req *Request, next StreamHandler) iter.Seq2[*Response, error] {
	return func(yield func(*Response, error) bool) {
		support := m.newToolSupport(len(req.Tools))

		if support.ShouldReturnDirect(req.Messages) {
			yield(support.BuildReturnDirectResponse(req.Messages))
			return
		}

		support.Register(req.Tools...)

		// HITL resume: continue from the conversation tail's unanswered tool
		// calls (a prior segment interrupted, its conversation fed back) —
		// execute only the pending calls, no model re-call. See executeCall.
		if assistant, done, pending := trailingPendingToolCalls(req.Messages); assistant != nil {
			m.resumeStreamRound(ctx, req, assistant, done, pending, next, support, yield, toolLoopState{iteration: priorModelRounds(req.Messages)})
			return
		}

		m.executeStreamRecursively(ctx, req, next, support, yield, toolLoopState{iteration: 1})
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
func (m *ToolMiddleware) executeStreamRecursively(ctx context.Context, req *Request, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool, state toolLoopState) {
	if state.iteration > m.maxIterations {
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
		if nudgeReq, ok, err := m.maybeNudgeEmpty(req, resp, state); err != nil {
			yield(nil, err)
		} else if ok {
			st := state.next()
			st.emptyRetried = true
			m.executeStreamRecursively(ctx, nudgeReq, next, support, yield, st)
		}
		return
	}

	result, err := support.InvokeToolCalls(ctx, req, resp)
	if err != nil {
		yield(nil, err)
		return
	}

	if result.interrupt != nil {
		// HITL: a tool call interrupted the round. Propagate a
		// *ToolLoopInterrupted carrying the resumable conversation so the
		// caller can save it, park, and resume. The round's assistant deltas
		// were already streamed above; on resume the loop re-enters at the
		// pending tool calls, not the model.
		yield(nil, m.wrapInterrupt(resp.Result.AssistantMessage, result.interrupt.done, result.interrupt.cause))
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
	m.executeStreamRecursively(ctx, nextReq, next, support, yield, state.next())
}

// maybeNudgeEmpty decides whether to re-prompt after an empty model reply.
// It returns (nextRequest, true, nil) when the empty-response feedback is
// enabled, hasn't been spent yet, and the response is genuinely empty;
// (nil, false, nil) otherwise.
func (m *ToolMiddleware) maybeNudgeEmpty(req *Request, resp *Response, state toolLoopState) (*Request, bool, error) {
	if !m.feedbackEmpty || state.emptyRetried || !resp.isEmpty() {
		return nil, false, nil
	}
	next, err := req.continueWith(resp.Result.AssistantMessage, NewUserMessage(emptyResponseNudge))
	if err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// isEmpty reports whether resp carries a real assistant turn with
// neither tool calls nor non-whitespace text. A nil result is treated as
// "not nudgeable" (there is no assistant message to append), so the loop
// returns it unchanged.
func (resp *Response) isEmpty() bool {
	if resp == nil || resp.Result == nil {
		return false
	}
	am := resp.Result.AssistantMessage
	if am == nil {
		return false
	}
	return !am.HasToolCalls() && strings.TrimSpace(am.JoinedText()) == ""
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
