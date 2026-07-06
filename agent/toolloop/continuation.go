package toolloop

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
)

// emptyResponseNudge is the follow-up prompt sent when a model returns an
// empty reply and [Config.FeedbackOnEmptyResponse] is enabled.
const emptyResponseNudge = "Your previous reply was empty. Please provide a complete answer, or call one of the available tools."

// applyBeforeRound appends any messages the BeforeRound hook supplies to a
// continuation request (after the tool result that round carries) — the seam
// for injecting a turn into a running loop. A nil hook or empty return leaves
// the request untouched. Options / Tools / Params are carried over unchanged.
func (m *middleware) applyBeforeRound(ctx context.Context, next *chat.Request) (*chat.Request, error) {
	if m.beforeRound == nil {
		return next, nil
	}
	extra := m.beforeRound(ctx)
	if len(extra) == 0 {
		return next, nil
	}
	return continueRequest(next, extra...)
}

// maybeNudgeEmpty decides whether to re-prompt after an empty model reply.
// It returns (nextRequest, true, nil) when the empty-response feedback is
// enabled, hasn't been spent yet, and the response is genuinely empty;
// (nil, false, nil) otherwise.
func (m *middleware) maybeNudgeEmpty(req *chat.Request, resp *chat.Response, state loopState) (*chat.Request, bool, error) {
	if !m.feedbackEmpty || state.emptyRetried || !resp.IsEmpty() {
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
