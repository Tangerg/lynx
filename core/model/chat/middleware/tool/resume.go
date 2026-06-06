package tool

import (
	"context"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
)

// buildInterruptResponse assembles the FinishReasonInterrupt response the loop
// hands back when a round halts for human input (HITL): the round's assistant
// tool-call message plus a tool message with the results already produced this
// round (nil when none ran yet). The caller captures this tail, parks the run,
// and feeds it back on resume so the loop continues AT the still-pending calls
// — never re-invoking the model. It mirrors return-direct's shape exactly,
// differing only in the finish reason.
func buildInterruptResponse(assistant *chat.AssistantMessage, done []*chat.ToolReturn) (*chat.Response, error) {
	result, err := chat.NewResult(assistant, &chat.ResultMetadata{FinishReason: chat.FinishReasonInterrupt})
	if err != nil {
		return nil, err
	}
	if len(done) > 0 {
		if tm, e := chat.NewToolMessage(done); e == nil {
			result.ToolMessage = tm
		}
	}
	return chat.NewResponse(result, &chat.ResponseMetadata{})
}

// trailingPendingToolCalls inspects the conversation tail for a resume point:
// an assistant message whose tool calls are not yet fully answered by a
// following tool message. It returns that assistant message, the results
// already produced (partial, in call order), and the calls still pending (in
// the assistant's order). When the tail is not a resumable point — no trailing
// assistant tool calls, or every call already answered — it returns
// (nil, nil, nil) and the loop starts a normal model round.
//
// This is what makes resume conversation-driven: a turn parked mid-round is
// fed its tail back; the shape alone tells the loop to execute the remaining
// calls rather than ask the model again. It handles both flavors uniformly —
// a pending call with no result is executed inline (e.g. a HITL-gated tool now
// approved), while a pending call whose result the host already supplied in
// the fed-back tool message is taken as-is (client-side / external tools).
func trailingPendingToolCalls(msgs []chat.Message) (assistant *chat.AssistantMessage, done []*chat.ToolReturn, pending []*chat.ToolCallPart) {
	if len(msgs) == 0 {
		return nil, nil, nil
	}

	var partial *chat.ToolMessage
	switch last := msgs[len(msgs)-1].(type) {
	case *chat.ToolMessage:
		partial = last
		if len(msgs) < 2 {
			return nil, nil, nil
		}
		am, ok := msgs[len(msgs)-2].(*chat.AssistantMessage)
		if !ok || !am.HasToolCalls() {
			return nil, nil, nil
		}
		assistant = am
	case *chat.AssistantMessage:
		if !last.HasToolCalls() {
			return nil, nil, nil
		}
		assistant = last
	default:
		return nil, nil, nil
	}

	answered := make(map[string]*chat.ToolReturn)
	if partial != nil {
		for _, ret := range partial.ToolReturns {
			answered[ret.ID] = ret
		}
	}

	for _, call := range assistant.CollectToolCalls() {
		if ret, ok := answered[call.ID]; ok {
			done = append(done, ret)
			continue
		}
		pending = append(pending, call)
	}
	if len(pending) == 0 {
		return nil, nil, nil // fully answered → not a resume point
	}
	return assistant, done, pending
}

// mergeRoundReturns orders the round's tool returns to match the assistant's
// tool-call order, drawing each from the already-done set or the
// freshly-produced set. Keeps tool_call_id ↔ result correlation intact for the
// next model round.
func mergeRoundReturns(calls []*chat.ToolCallPart, done, fresh []*chat.ToolReturn) []*chat.ToolReturn {
	byID := make(map[string]*chat.ToolReturn, len(done)+len(fresh))
	for _, r := range done {
		byID[r.ID] = r
	}
	for _, r := range fresh {
		byID[r.ID] = r
	}
	out := make([]*chat.ToolReturn, 0, len(calls))
	for _, call := range calls {
		if r, ok := byID[call.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// priorModelRounds counts the assistant messages in a resumed conversation —
// the model rounds already spent — so the resumed loop keeps counting toward
// the iteration cap instead of restarting at 1.
func priorModelRounds(msgs []chat.Message) int {
	n := 0
	for _, msg := range msgs {
		if _, ok := msg.(*chat.AssistantMessage); ok {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

// allReturnDirect reports whether every tool referenced in returns is
// registered AND return-direct — the resume-path analog of the allReturnDirect
// bit invokeToolCalls computes inline.
func (s *support) allReturnDirect(returns []*chat.ToolReturn) bool {
	for _, ret := range returns {
		t, exists := s.registry.find(ret.Name)
		if !exists || !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// resumeCall runs the still-pending tool calls of a resumed round on the
// synchronous path, then re-interrupts, returns direct, or continues the loop
// at the next model round. It never re-invokes the model for the resumed round
// — the assistant message is already known.
func (m *middleware) resumeCall(ctx context.Context, req *chat.Request, assistant *chat.AssistantMessage, done []*chat.ToolReturn, pending []*chat.ToolCallPart, next chat.CallHandler, support *support, state loopState) (*chat.Response, error) {
	res, err := support.invoker.invokeToolCalls(ctx, pending)
	if err != nil {
		return nil, err
	}
	if res.interrupt != nil {
		// Another call in the same round halted: fold the results so far into
		// the round's done-set and surface the interrupt tail again.
		merged := append(slices.Clone(done), res.interrupt.done...)
		resp, e := buildInterruptResponse(assistant, merged)
		if e != nil {
			return nil, e
		}
		return resp, res.interrupt.cause
	}

	full := mergeRoundReturns(assistant.CollectToolCalls(), done, res.toolMessage.ToolReturns)
	toolMsg, err := chat.NewToolMessage(full)
	if err != nil {
		return nil, err
	}
	if support.allReturnDirect(full) {
		return buildResumedReturnResponse(assistant, toolMsg)
	}
	nextReq, err := buildResumedContinueRequest(req, assistant, toolMsg)
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, support, state.next())
}

// resumeStream is the streaming analog of [resumeCall]. It surfaces the
// resumed round's tool message to the stream (so the wire timeline + caller's
// per-round budget boundary see it) before continuing.
func (m *middleware) resumeStream(ctx context.Context, req *chat.Request, assistant *chat.AssistantMessage, done []*chat.ToolReturn, pending []*chat.ToolCallPart, next chat.StreamHandler, support *support, yield func(*chat.Response, error) bool, state loopState) {
	res, err := support.invoker.invokeToolCalls(ctx, pending)
	if err != nil {
		yield(nil, err)
		return
	}
	if res.interrupt != nil {
		merged := append(slices.Clone(done), res.interrupt.done...)
		if resp, e := buildInterruptResponse(assistant, merged); e != nil {
			yield(nil, e)
		} else {
			yield(resp, nil)
			yield(nil, res.interrupt.cause)
		}
		return
	}

	full := mergeRoundReturns(assistant.CollectToolCalls(), done, res.toolMessage.ToolReturns)
	toolMsg, err := chat.NewToolMessage(full)
	if err != nil {
		yield(nil, err)
		return
	}
	if toolResp, e := newToolMessageResponse(toolMsg); e == nil && !yield(toolResp, nil) {
		return
	}
	if support.allReturnDirect(full) {
		yield(buildResumedReturnResponse(assistant, toolMsg))
		return
	}
	nextReq, err := buildResumedContinueRequest(req, assistant, toolMsg)
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, support, yield, state.next())
}

// buildResumedContinueRequest assembles the next model request after a resumed
// round completes: the turn's system header, the assistant tool-call message,
// and the assembled tool results, carrying the live request's options / tools
// / params. The prior conversation is NOT re-sent (the memory middleware
// splices stored history back in); the assistant + tool results travel
// together so the memory layer persists the exchange atomically — the
// interrupting round's assistant was deliberately never persisted, so this is
// where the (assistant, tool) pair lands.
func buildResumedContinueRequest(req *chat.Request, assistant *chat.AssistantMessage, toolMsg *chat.ToolMessage) (*chat.Request, error) {
	msgs := append(systemMessages(req.Messages), chat.Message(assistant), chat.Message(toolMsg))
	next, err := chat.NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}

// buildResumedReturnResponse wraps a resumed round's tool message as the final
// response when every tool in the round is return-direct.
func buildResumedReturnResponse(assistant *chat.AssistantMessage, toolMsg *chat.ToolMessage) (*chat.Response, error) {
	result, err := chat.NewResult(assistant, &chat.ResultMetadata{FinishReason: chat.FinishReasonReturnDirect})
	if err != nil {
		return nil, err
	}
	result.ToolMessage = toolMsg
	return chat.NewResponse(result, &chat.ResponseMetadata{})
}
