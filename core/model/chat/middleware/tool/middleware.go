package tool

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// DefaultMaxIterations bounds the self-driving tool loop. A model that
// keeps requesting tools — or a buggy tool whose result always re-triggers
// a call — would otherwise spin forever; the cap turns that into a
// [MaxIterationsError] instead. Mirrors embabel's
// MaxIterationsExceededException.
const DefaultMaxIterations = 50

// emptyResponseNudge is the follow-up prompt sent when a model returns an
// empty reply and [LoopConfig.FeedbackOnEmptyResponse] is enabled.
const emptyResponseNudge = "Your previous reply was empty. Please provide a complete answer, or call one of the available tools."

// LoopConfig tunes [NewMiddleware]. Every field is optional; the
// zero value yields the default loop (cap = [DefaultMaxIterations]).
//
// Tool-failure handling is NOT configurable: a tool error that isn't a
// control-flow signal (ctx cancel / a chat.ToolHalt (abort or HITL interrupt)) is
// always fed back to the model as a tool result so it can recover, and an
// unregistered tool is always answered with an error result rather than
// aborting the run. That recovery is the framework default; a tool author
// chooses the outcome at the source (fold the failure into the result string,
// or return an ordinary error and let the loop wrap it). See [chat.Tool.Call].
type LoopConfig struct {
	// MaxIterations caps the number of model calls the tool loop makes.
	// <= 0 falls back to [DefaultMaxIterations].
	MaxIterations int

	// FeedbackOnEmptyResponse, when true, answers an empty model reply (no
	// text and no tool calls) with a nudge and re-prompts the model once
	// instead of returning the empty reply. Default off. Unlike tool-failure
	// recovery this is a genuine behavioral choice, not error handling, so it
	// stays opt-in.
	FeedbackOnEmptyResponse bool
}

// MaxIterationsError is returned when the tool-calling loop exceeds its
// configured iteration cap. Callers can detect it with [errors.As].
type MaxIterationsError struct {
	Limit int
}

func (e *MaxIterationsError) Error() string {
	return fmt.Sprintf("tool: loop exceeded %d iterations without a final reply", e.Limit)
}

// middleware turns the model handler into a self-driving tool-calling
// loop. When the LLM emits tool calls the middleware executes them via
// the [support] machinery and re-prompts the model with the results,
// repeating until the model produces a regular reply, every tool is
// configured for direct return, or the iteration cap is hit.
//
// Use it via [NewMiddleware], which returns both call and stream
// halves so a single registration covers both paths.
//
// Example:
//
//	callMW, streamMW := tool.NewMiddleware()
//	resp, err := client.Chat().
//	    WithMiddlewares(callMW, streamMW).
//	    WithTools(myTool).
//	    Call().Response(ctx)
type middleware struct {
	maxIterations int
	feedbackEmpty bool
}

// NewMiddleware constructs the tool-calling middleware pair. Pass an
// optional [LoopConfig] to tune the loop; omit it for defaults.
func NewMiddleware(config ...LoopConfig) (chat.CallMiddleware, chat.StreamMiddleware) {
	var cfg LoopConfig
	if len(config) > 0 {
		cfg = config[0]
	}

	maxIterations := cfg.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	mw := &middleware{
		maxIterations: maxIterations,
		feedbackEmpty: cfg.FeedbackOnEmptyResponse,
	}
	return mw.wrapCallHandler, mw.wrapStreamHandler
}

// loopState carries the per-loop bookkeeping threaded through the
// recursion: the 1-based model-call count and whether the one-shot
// empty-response nudge has already been spent.
type loopState struct {
	iteration    int
	emptyRetried bool
}

func (s loopState) next() loopState {
	s.iteration++
	return s
}

// wrapCallHandler is the call-side adapter — turns the middleware body
// into a [chat.CallHandler] decorator.
func (m *middleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

// wrapStreamHandler is the stream-side adapter.
func (m *middleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}

// newSupport builds the per-loop support. Tool-failure recovery
// (unknown-tool + recoverable-error feedback) is the unconditional default in
// [callInvoker], so there's nothing to configure here.
func (m *middleware) newSupport(toolCount int) *support {
	return newSupport(toolCount)
}

// executeCall is the synchronous entry point: short-circuit when prior
// messages already indicate a direct return; otherwise enter the
// recursive call/tool loop.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	support := m.newSupport(len(req.Tools))

	if support.shouldReturnDirect(req.Messages) {
		return support.buildReturnDirectResponse(req.Messages)
	}

	support.register(req.Tools...)

	return m.executeCallRecursively(ctx, req, next, support, loopState{iteration: 1})
}

// executeCallRecursively runs one round of model + tool execution. If
// the model asks for tools and the tools want LLM follow-up, the
// function re-prompts and recurses. state.iteration is the 1-based
// model-call count; exceeding maxIterations aborts with a
// [MaxIterationsError].
func (m *middleware) executeCallRecursively(ctx context.Context, req *chat.Request, next chat.CallHandler, support *support, state loopState) (*chat.Response, error) {
	if state.iteration > m.maxIterations {
		return nil, &MaxIterationsError{Limit: m.maxIterations}
	}

	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}

	shouldInvoke, err := support.shouldInvokeToolCalls(resp)
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

	result, err := support.invokeToolCalls(ctx, req, resp)
	if err != nil {
		// A control-flow signal (HITL interrupt, abort, ctx cancel) propagates
		// unchanged so an outer layer can park or fail the run; on a HITL
		// interrupt the run resumes by re-running this turn.
		return nil, err
	}

	if result.shouldReturn() {
		return result.buildReturnResponse()
	}

	nextReq, err := result.buildContinueRequest()
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, support, state.next())
}

// executeStream is the streaming entry point. Same shape as executeCall
// but delivers chunks through the iterator while accumulating them so
// the tool-calling loop can inspect a complete response when the stream
// closes.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		support := m.newSupport(len(req.Tools))

		if support.shouldReturnDirect(req.Messages) {
			yield(support.buildReturnDirectResponse(req.Messages))
			return
		}

		support.register(req.Tools...)

		m.executeStreamRecursively(ctx, req, next, support, yield, loopState{iteration: 1})
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
func (m *middleware) executeStreamRecursively(ctx context.Context, req *chat.Request, next chat.StreamHandler, support *support, yield func(*chat.Response, error) bool, state loopState) {
	if state.iteration > m.maxIterations {
		yield(nil, &MaxIterationsError{Limit: m.maxIterations})
		return
	}

	accumulator := chat.NewResponseAccumulator()

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
	shouldInvoke, err := support.shouldInvokeToolCalls(resp)
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

	result, err := support.invokeToolCalls(ctx, req, resp)
	if err != nil {
		// A control-flow signal (HITL interrupt, abort, ctx cancel) propagates
		// unchanged. The round's assistant deltas were already streamed above;
		// on a HITL interrupt the run resumes by re-running this turn.
		yield(nil, err)
		return
	}

	if result.shouldReturn() {
		yield(result.buildReturnResponse())
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

	nextReq, err := result.buildContinueRequest()
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
func (m *middleware) maybeNudgeEmpty(req *chat.Request, resp *chat.Response, state loopState) (*chat.Request, bool, error) {
	if !m.feedbackEmpty || state.emptyRetried || !isEmpty(resp) {
		return nil, false, nil
	}
	next, err := continueRequest(req, resp.Result.AssistantMessage, chat.NewUserMessage(emptyResponseNudge))
	if err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// continueRequest assembles a follow-up request carrying the live request's
// messages plus any extra messages appended, with options / tools / params
// cloned from the original.
func continueRequest(req *chat.Request, extra ...chat.Message) (*chat.Request, error) {
	msgs := append(slices.Clone(req.Messages), extra...)
	next, err := chat.NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}

// isEmpty reports whether resp carries a real assistant turn with
// neither tool calls nor non-whitespace text. A nil result is treated as
// "not nudgeable" (there is no assistant message to append), so the loop
// returns it unchanged.
func isEmpty(resp *chat.Response) bool {
	if resp == nil || resp.Result == nil {
		return false
	}
	am := resp.Result.AssistantMessage
	if am == nil {
		return false
	}
	return !am.HasToolCalls() && strings.TrimSpace(am.JoinedText()) == ""
}

// newToolMessageResponse wraps a [*chat.ToolMessage] in a [*chat.Response] whose
// Result.ToolMessage is set and Result.AssistantMessage is nil — the
// discriminator that distinguishes tool-injection deltas from model
// output deltas on the stream.
func newToolMessageResponse(tm *chat.ToolMessage) (*chat.Response, error) {
	result := &chat.Result{
		ToolMessage: tm,
		Metadata:    &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
	}
	return chat.NewResponse(result, &chat.ResponseMetadata{})
}
