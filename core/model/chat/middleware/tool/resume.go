package tool

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
)

// toolRoundResponse wraps an assistant turn (and the round's tool message, when
// present) as a [*chat.Response] carrying the given finish reason — the common
// shape of the loop's synthetic terminal responses (return-direct and HITL
// interrupt). The response-level metadata is empty; only the result-level
// finish reason distinguishes them.
func toolRoundResponse(assistant *chat.AssistantMessage, toolMsg *chat.ToolMessage, reason chat.FinishReason) (*chat.Response, error) {
	result, err := chat.NewResult(assistant, &chat.ResultMetadata{FinishReason: reason})
	if err != nil {
		return nil, err
	}
	if toolMsg != nil {
		result.ToolMessage = toolMsg
	}
	return chat.NewResponse(result, &chat.ResponseMetadata{})
}

// buildInterruptResponse assembles the FinishReasonInterrupt response the loop
// hands back when a round halts for human input (HITL): the round's assistant
// tool-call message plus a tool message with the results already produced this
// round (nil when none ran yet). The caller captures this tail, parks the run,
// and feeds it back on resume so the loop continues AT the still-pending calls
// — never re-invoking the model. It mirrors return-direct's shape exactly,
// differing only in the finish reason.
//
// The first interrupt of a round arrives with just that round's done-set;
// a re-interrupt during a resumed round folds the prior round's done-set in
// first (see [middleware.resumeCall]). Both flow through
// [middleware.interruptOutcome], which picks park-vs-tail.
func buildInterruptResponse(assistant *chat.AssistantMessage, done []*chat.ToolReturn) (*chat.Response, error) {
	var toolMsg *chat.ToolMessage
	if len(done) > 0 {
		if tm, err := chat.NewToolMessage(done); err == nil {
			toolMsg = tm
		}
	}
	return toolRoundResponse(assistant, toolMsg, chat.FinishReasonInterrupt)
}

// resumePoint is a parsed HITL resume tail: the round's assistant tool-call
// message, the results already produced (done, in call order), the calls still
// pending (in the assistant's order), and how many model rounds were already
// spent. It exists so the resume invariant — execute only the pending calls,
// never re-invoke the model for this round — has a name, and so the call and
// stream paths drive it through one shape instead of threading the same four
// values through their parameter lists.
type resumePoint struct {
	assistant   *chat.AssistantMessage
	done        []*chat.ToolReturn
	pending     []*chat.ToolCallPart
	priorRounds int
}

// parseResumePoint inspects the conversation tail for a resume point: an
// assistant message whose tool calls are not yet fully answered by a following
// tool message. It returns (point, true) when the tail is resumable; otherwise
// (nil, false) and the loop starts a normal model round — when the tail is not
// trailing assistant tool calls, or every call is already answered.
//
// This is what makes resume conversation-driven: a turn parked mid-round is
// fed its tail back; the shape alone tells the loop to execute the remaining
// calls rather than ask the model again. It handles both flavors uniformly —
// a pending call with no result is executed inline (e.g. a HITL-gated tool now
// approved), while a pending call whose result the host already supplied in
// the fed-back tool message is taken as-is (client-side / external tools).
func parseResumePoint(msgs []chat.Message) (*resumePoint, bool) {
	if len(msgs) == 0 {
		return nil, false
	}

	var (
		assistant *chat.AssistantMessage
		partial   *chat.ToolMessage
	)
	switch last := msgs[len(msgs)-1].(type) {
	case *chat.ToolMessage:
		partial = last
		if len(msgs) < 2 {
			return nil, false
		}
		am, ok := msgs[len(msgs)-2].(*chat.AssistantMessage)
		if !ok || !am.HasToolCalls() {
			return nil, false
		}
		assistant = am
	case *chat.AssistantMessage:
		if !last.HasToolCalls() {
			return nil, false
		}
		assistant = last
	default:
		return nil, false
	}

	answered := make(map[string]*chat.ToolReturn)
	if partial != nil {
		for _, ret := range partial.ToolReturns {
			answered[ret.ID] = ret
		}
	}

	var (
		done    []*chat.ToolReturn
		pending []*chat.ToolCallPart
	)
	for _, call := range assistant.CollectToolCalls() {
		if ret, ok := answered[call.ID]; ok {
			done = append(done, ret)
			continue
		}
		pending = append(pending, call)
	}
	if len(pending) == 0 {
		return nil, false // fully answered → not a resume point
	}

	return &resumePoint{
		assistant:   assistant,
		done:        done,
		pending:     pending,
		priorRounds: priorModelRounds(msgs),
	}, true
}

// merge orders this round's tool returns to match the assistant's tool-call
// order, drawing each from the already-done set or the freshly-produced set.
// Keeps tool_call_id ↔ result correlation intact for the next model round.
func (p *resumePoint) merge(fresh []*chat.ToolReturn) []*chat.ToolReturn {
	calls := p.assistant.CollectToolCalls()
	byID := make(map[string]*chat.ToolReturn, len(p.done)+len(fresh))
	for _, r := range p.done {
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
func (i *invoker) allReturnDirect(returns []*chat.ToolReturn) bool {
	for _, ret := range returns {
		t, exists := i.registry.find(ret.Name)
		if !exists || !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// resumeCall runs the still-pending tool calls of a resumed round on the
// synchronous path, then re-interrupts, returns direct, or continues the loop
// at the next model round. It never re-invokes the model for the resumed round
// — the assistant message is already known (carried by point).
func (m *middleware) resumeCall(ctx context.Context, req *chat.Request, point *resumePoint, next chat.CallHandler, inv *invoker) (*chat.Response, error) {
	res, err := inv.invokeToolCalls(ctx, point.pending)
	if err != nil {
		return nil, err
	}
	if res.interrupt != nil {
		// Another call in the same round halted. Fold the prior
		// done-set in so the next resume sees every result of the
		// round, then park or hand the tail back.
		merged := append(slices.Clone(point.done), res.interrupt.done...)
		tail, e := m.interruptOutcome(ctx, req, point.assistant, merged)
		if e != nil {
			return nil, e
		}
		return tail, res.interrupt.cause
	}

	full := point.merge(res.toolMessage.ToolReturns)
	toolMsg, err := chat.NewToolMessage(full)
	if err != nil {
		return nil, err
	}
	if inv.allReturnDirect(full) {
		return toolRoundResponse(point.assistant, toolMsg, chat.FinishReasonReturnDirect)
	}
	nextReq, err := nextRoundRequest(req, point.assistant, toolMsg)
	if err != nil {
		return nil, err
	}
	return m.executeCallRecursively(ctx, nextReq, next, inv, loopState{iteration: point.priorRounds}.next())
}

// resumeStream is the streaming analog of [resumeCall]. It surfaces the
// resumed round's tool message to the stream (so the wire timeline + caller's
// per-round budget boundary see it) before continuing.
func (m *middleware) resumeStream(ctx context.Context, req *chat.Request, point *resumePoint, next chat.StreamHandler, inv *invoker, yield func(*chat.Response, error) bool) {
	res, err := inv.invokeToolCalls(ctx, point.pending)
	if err != nil {
		yield(nil, err)
		return
	}
	if res.interrupt != nil {
		// Re-interrupt during a resumed round — same fold-and-deliver
		// as [middleware.resumeCall], on the stream protocol.
		merged := &roundInterrupt{
			done:  append(slices.Clone(point.done), res.interrupt.done...),
			cause: res.interrupt.cause,
		}
		m.yieldInterrupt(ctx, req, point.assistant, merged, yield)
		return
	}

	full := point.merge(res.toolMessage.ToolReturns)
	toolMsg, err := chat.NewToolMessage(full)
	if err != nil {
		yield(nil, err)
		return
	}
	if toolResp, e := newToolMessageResponse(toolMsg); e == nil && !yield(toolResp, nil) {
		return
	}
	if inv.allReturnDirect(full) {
		yield(toolRoundResponse(point.assistant, toolMsg, chat.FinishReasonReturnDirect))
		return
	}
	nextReq, err := nextRoundRequest(req, point.assistant, toolMsg)
	if err != nil {
		yield(nil, err)
		return
	}
	m.executeStreamRecursively(ctx, nextReq, next, inv, yield, loopState{iteration: point.priorRounds}.next())
}
