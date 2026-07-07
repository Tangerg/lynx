package toolloop

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/core/model/chat"
)

func (m *middleware) wrapStreamHandler(next chat.StreamHandler) chat.StreamHandler {
	return chat.StreamHandlerFunc(func(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
		return m.executeStream(ctx, req, next)
	})
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
