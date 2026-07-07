package toolloop

import (
	"context"

	"github.com/Tangerg/lynx/core/model/chat"
)

func (m *middleware) wrapCallHandler(next chat.CallHandler) chat.CallHandler {
	return chat.CallHandlerFunc(func(ctx context.Context, req *chat.Request) (*chat.Response, error) {
		return m.executeCall(ctx, req, next)
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
