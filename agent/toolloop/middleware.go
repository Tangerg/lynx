package toolloop

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"
)

// DefaultMaxIterations bounds the self-driving tool loop. A model that
// keeps requesting tools — or a buggy tool whose result always re-triggers
// a call — would otherwise spin forever; the cap turns that into a
// [MaxIterationsError] instead.
const DefaultMaxIterations = 50

// Config tunes [NewMiddleware]. Every field is optional; the zero
// value yields the default loop (cap = [DefaultMaxIterations]).
//
// Tool-failure handling is NOT configurable: a tool error that isn't a
// control-flow signal (ctx cancel / a Halt (abort or HITL interrupt)) is
// always fed back to the model as a tool result so it can recover, and an
// unregistered tool is always answered with an error result rather than
// aborting the run. That recovery is the framework default; a tool author
// chooses the outcome at the source (fold the failure into the result string,
// or return an ordinary error and let the loop wrap it). See [chat.Tool.Call].
type Config struct {
	// MaxIterations caps the number of model calls the tool loop makes.
	// <= 0 falls back to [DefaultMaxIterations].
	MaxIterations int

	// FeedbackOnEmptyResponse, when true, answers an empty model reply (no
	// text and no tool calls) with a nudge and re-prompts the model once
	// instead of returning the empty reply. Default off. Unlike tool-failure
	// recovery this is a genuine behavioral choice, not error handling, so it
	// stays opt-in.
	FeedbackOnEmptyResponse bool

	// ParkStore, when non-nil, persists interrupted tool rounds so the
	// engine never sees a [FinishReasonInterrupt] chunk — the middleware
	// saves the park state on interrupt and restores it on resume, both
	// transparent to the caller. nil (the zero-value default) selects the
	// conversation-tail design instead:
	// [buildInterruptResponse] hands the interrupted round back as a
	// [FinishReasonInterrupt] response whose tail the caller re-feeds to
	// resume.
	ParkStore ParkStore

	// LoopDetection, when non-nil, enables the repeated-tool-round
	// detector: a round whose (tool, arguments, result) signature recurs
	// past the configured threshold halts the loop with a
	// [LoopDetectedError] — well before [MaxIterations]. nil (the
	// zero-value default) disables it, leaving the loop bounded only by
	// MaxIterations. See [LoopDetectionConfig].
	LoopDetection *LoopDetectionConfig

	// BeforeRound, when non-nil, is invoked before each CONTINUATION model
	// round — after a tool result is fed back, before the next model call;
	// never before the FIRST round. Any messages it returns are appended to
	// that round's request (after the tool result), so a caller can inject a
	// turn into a running loop without restarting it — e.g. "mid-run steering",
	// where a user message reaches the model on the next round. Returning nil
	// (the common case) leaves the round unchanged. The hook must not block.
	// Default nil.
	BeforeRound func(ctx context.Context) []chat.Message
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
// the [invoker] and re-prompts the model with the results,
// repeating until the model produces a regular reply, every tool is
// configured for direct return, or the iteration cap is hit.
//
// Use it via [NewMiddleware], which returns both call and stream
// halves so a single registration covers both paths.
//
// Example:
//
//	callMW, streamMW := toolloop.NewMiddleware()
//	resp, err := client.Chat().
//	    WithCallMiddlewares(callMW).
//	    WithStreamMiddlewares(streamMW).
//	    WithTools(myTool).
//	    Call().Response(ctx)
type middleware struct {
	maxIterations int
	feedbackEmpty bool
	parkStore     ParkStore
	loopDetection *LoopDetectionConfig
	beforeRound   func(ctx context.Context) []chat.Message
}

// NewMiddleware constructs the tool-calling middleware pair. Pass an
// optional [Config] to tune the loop; omit it for defaults.
func NewMiddleware(config ...Config) (chat.CallMiddleware, chat.StreamMiddleware) {
	var cfg Config
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
		parkStore:     cfg.ParkStore,
		loopDetection: cfg.LoopDetection,
		beforeRound:   cfg.BeforeRound,
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

// nudged returns the next-iteration state with the one-shot
// empty-response retry marked as spent, so the nudge fires at most once
// per loop.
func (s loopState) nudged() loopState {
	s = s.next()
	s.emptyRetried = true
	return s
}

func (m *middleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
	})
}

func (m *middleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
}

// executeCall is the synchronous entry point: short-circuit when prior
// messages already indicate a direct return; otherwise enter the
// recursive call/tool loop.
func (m *middleware) executeCall(ctx context.Context, req *chat.Request, next chat.CallHandler) (*chat.Response, error) {
	inv := newInvoker(len(req.Tools))

	if inv.shouldReturnDirect(req.Messages) {
		return inv.buildReturnDirectResponse(req.Messages)
	}

	inv.register(req.Tools...)

	req, err := m.restorePark(ctx, req)
	if err != nil {
		return nil, err
	}

	// HITL resume: when the conversation tail is an assistant turn whose tool
	// calls aren't fully answered (a prior round halted for human input and its
	// tail was fed back), execute the still-pending calls and continue —
	// without re-invoking the model for the already-produced assistant.
	if point, ok := parseResumePoint(req.Messages); ok {
		return m.resumeCall(ctx, req, point, next, inv)
	}

	return m.executeCallRecursively(ctx, req, next, inv, newLoopDetector(m.loopDetection), loopState{iteration: 1})
}

// executeCallRecursively runs one round of model + tool execution. If
// the model asks for tools and the tools want LLM follow-up, the
// function re-prompts and recurses. state.iteration is the 1-based
// model-call count; exceeding maxIterations aborts with a
// [MaxIterationsError].
func (m *middleware) executeCallRecursively(ctx context.Context, req *chat.Request, next chat.CallHandler, inv *invoker, det *loopDetector, state loopState) (*chat.Response, error) {
	if state.iteration > m.maxIterations {
		return nil, &MaxIterationsError{Limit: m.maxIterations}
	}

	resp, err := next.Call(ctx, req)
	if err != nil {
		return nil, err
	}

	if !inv.canInvokeToolCalls(resp) {
		if nudgeReq, ok, nudgeErr := m.maybeNudgeEmpty(req, resp, state); nudgeErr != nil {
			return nil, nudgeErr
		} else if ok {
			return m.executeCallRecursively(ctx, nudgeReq, next, inv, det, state.nudged())
		}
		return resp, nil
	}

	result, err := inv.invoke(ctx, req, resp)
	if err != nil {
		// A fatal control-flow signal (abort / ctx cancel) propagates unchanged.
		return nil, err
	}

	if result.interrupt != nil {
		// HITL: a tool halted the round — park or hand the tail back.
		tail, e := m.interruptOutcome(ctx, req, resp.Result.AssistantMessage, result.interrupt.done)
		if e != nil {
			return nil, e
		}
		return tail, result.interrupt.cause
	}

	if result.shouldReturn() {
		return result.buildReturnResponse()
	}

	nudge := false
	if det != nil {
		halt, n := det.observe(roundSignature(resp.Result.AssistantMessage.CollectToolCalls(), result.toolMessage))
		if halt != nil {
			return nil, halt
		}
		nudge = n
	}

	nextReq, err := result.buildContinueRequest()
	if err != nil {
		return nil, err
	}
	if nudge {
		if nextReq, err = continueRequest(nextReq, chat.NewUserMessage(loopNudge)); err != nil {
			return nil, err
		}
	}
	nextReq, err = m.applyBeforeRound(ctx, nextReq)
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, inv, det, state.next())
}

// executeStream is the streaming entry point. Same shape as executeCall
// but delivers chunks through the iterator while accumulating them so
// the tool-calling loop can inspect a complete response when the stream
// closes.
func (m *middleware) executeStream(ctx context.Context, req *chat.Request, next chat.StreamHandler) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		inv := newInvoker(len(req.Tools))

		if inv.shouldReturnDirect(req.Messages) {
			yield(inv.buildReturnDirectResponse(req.Messages))
			return
		}

		inv.register(req.Tools...)

		req, err := m.restorePark(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}

		// HITL resume: continue from the conversation tail's unanswered tool
		// calls (a prior round halted, its tail fed back) — execute only the
		// pending calls, no model re-call. See executeCall.
		if point, ok := parseResumePoint(req.Messages); ok {
			m.resumeStream(ctx, req, point, next, inv, yield)
			return
		}

		m.executeStreamRecursively(ctx, req, next, inv, newLoopDetector(m.loopDetection), yield, loopState{iteration: 1})
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
func (m *middleware) executeStreamRecursively(ctx context.Context, req *chat.Request, next chat.StreamHandler, inv *invoker, det *loopDetector, yield func(*chat.Response, error) bool, state loopState) {
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
	if !inv.canInvokeToolCalls(resp) {
		if nudgeReq, ok, nudgeErr := m.maybeNudgeEmpty(req, resp, state); nudgeErr != nil {
			yield(nil, nudgeErr)
		} else if ok {
			m.executeStreamRecursively(ctx, nudgeReq, next, inv, det, yield, state.nudged())
		}
		return
	}

	result, err := inv.invoke(ctx, req, resp)
	if err != nil {
		// A fatal control-flow signal (abort / ctx cancel) propagates unchanged.
		yield(nil, err)
		return
	}

	if result.interrupt != nil {
		// HITL: a tool halted the round — park or stream the tail.
		m.yieldInterrupt(ctx, req, resp.Result.AssistantMessage, result.interrupt, yield)
		return
	}

	if result.shouldReturn() {
		yield(result.buildReturnResponse())
		return
	}

	// Tool round-trip is happening in the middle of the loop. Surface
	// the ToolMessage to the stream consumer so the on-the-wire
	// timeline matches the message history handed to the next
	// model turn.
	if result.toolMessage != nil {
		toolResp, wrapErr := newToolMessageResponse(result.toolMessage)
		if wrapErr == nil && !yield(toolResp, nil) {
			return
		}
	}

	nudge := false
	if det != nil {
		halt, n := det.observe(roundSignature(resp.Result.AssistantMessage.CollectToolCalls(), result.toolMessage))
		if halt != nil {
			yield(nil, halt)
			return
		}
		nudge = n
	}

	nextReq, err := result.buildContinueRequest()
	if err != nil {
		yield(nil, err)
		return
	}
	if nudge {
		if nextReq, err = continueRequest(nextReq, chat.NewUserMessage(loopNudge)); err != nil {
			yield(nil, err)
			return
		}
	}
	nextReq, err = m.applyBeforeRound(ctx, nextReq)
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, inv, det, yield, state.next())
}
