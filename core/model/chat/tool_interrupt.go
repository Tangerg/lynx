package chat

import (
	"context"
	"errors"
	"maps"
	"slices"
)

// interruptsToolLoop reports whether a tool error is a human-in-the-loop
// INTERRUPT — the loop must exit immediately and propagate it unchanged
// (no feedback to the model, no abort-masking) so the caller can park the
// run and gather input. A tool signals it by returning an error
// implementing ToolLoopInterrupt() bool returning true
// (agent/hitl.InterruptError does). Duck-typed so this package never
// imports agent — the one-way dependency holds.
//
// This is the Go-ecosystem interrupt model (LangGraph / eino /
// trpc-agent-go): the interrupt is an ordinary error carrying its payload;
// it bubbles to where it is handled, the payload is extracted, HITL is
// triggered, and on resume the human's response is fed back at the original
// call site.
func interruptsToolLoop(err error) bool {
	var i interface{ ToolLoopInterrupt() bool }
	return errors.As(err, &i) && i.ToolLoopInterrupt()
}

// ToolLoopInterrupted is the error the tool loop returns when a tool
// interrupts the run for human input. It carries the Conversation the loop
// had built when it interrupted — the prior turns, the assistant message
// that requested this round's tools, and (when some of the round's calls
// already ran) a tool message holding their results — i.e. everything
// needed to RESUME the turn by executing the still-pending tool calls,
// without re-invoking the model.
//
// The caller (the agent action) saves Conversation, parks the run on the
// underlying interrupt (Unwrap → the tool's interrupt error, which carries
// the user-facing payload + the resume awaitable), and on resume feeds
// Conversation back as the request messages. The loop then detects the
// trailing unanswered tool calls and resumes execution — the
// conversation-shape IS the checkpoint, matching the canonical
// interrupt/resume model.
type ToolLoopInterrupted struct {
	// Conversation is the message list to persist and feed back on resume.
	// Its tail is the assistant tool-call message, optionally followed by a
	// tool message with the results already produced this round.
	Conversation []Message

	// cause is the tool's interrupt error (carries the HITL payload). It is
	// reachable via errors.As through Unwrap.
	cause error
}

func (e *ToolLoopInterrupted) Error() string {
	return "chat: tool loop interrupted for human input: " + e.cause.Error()
}

// Unwrap exposes the underlying interrupt error so callers extract the
// payload / awaitable with errors.As.
func (e *ToolLoopInterrupted) Unwrap() error { return e.cause }

// toolRoundInterrupt is the partial result of a tool round that
// interrupted: the results already produced this invocation (done) and the
// interrupt error to propagate. The still-pending calls are derived from
// the assistant message at resume time (its tool calls minus the answered
// ones), so they are not carried here.
type toolRoundInterrupt struct {
	done  []*ToolReturn
	cause error
}

// interruptedConversation assembles the resumable conversation for a round
// that interrupted: the messages before the round, the assistant tool-call
// message, and — when some calls already ran — a tool message with their
// results. On resume [trailingPendingToolCalls] reads this shape to find
// the calls still to run.
func interruptedConversation(prior []Message, assistant *AssistantMessage, done []*ToolReturn) []Message {
	out := append(slices.Clone(prior), Message(assistant))
	if len(done) > 0 {
		if tm, err := NewToolMessage(done); err == nil {
			out = append(out, tm)
		}
	}
	return out
}

// trailingPendingToolCalls inspects the conversation tail for a resume
// point: an assistant message whose tool calls are not yet fully answered
// by a following tool message. It returns that assistant message, the
// results already produced (partial, in call order), and the calls still
// pending (in the assistant's order). When the tail is not a resumable
// point — no trailing assistant tool calls, or every call already answered
// — it returns (nil, nil, nil) and the loop starts a normal model round.
//
// This is what makes resume conversation-driven: a turn parked mid-round
// is fed its saved conversation back; the shape alone tells the loop to
// execute the remaining calls rather than ask the model again.
func trailingPendingToolCalls(msgs []Message) (assistant *AssistantMessage, done []*ToolReturn, pending []*ToolCallPart) {
	if len(msgs) == 0 {
		return nil, nil, nil
	}

	var partial *ToolMessage
	switch last := msgs[len(msgs)-1].(type) {
	case *ToolMessage:
		partial = last
		if len(msgs) < 2 {
			return nil, nil, nil
		}
		am, ok := msgs[len(msgs)-2].(*AssistantMessage)
		if !ok || !am.HasToolCalls() {
			return nil, nil, nil
		}
		assistant = am
	case *AssistantMessage:
		if !last.HasToolCalls() {
			return nil, nil, nil
		}
		assistant = last
	default:
		return nil, nil, nil
	}

	answered := make(map[string]*ToolReturn)
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

// mergeRoundReturns orders the round's tool returns to match the
// assistant's tool-call order, drawing each from the already-done set or
// the freshly-produced set. Keeps tool_call_id ↔ result correlation intact
// for the next model round.
func mergeRoundReturns(calls []*ToolCallPart, done, fresh []*ToolReturn) []*ToolReturn {
	byID := make(map[string]*ToolReturn, len(done)+len(fresh))
	for _, r := range done {
		byID[r.ID] = r
	}
	for _, r := range fresh {
		byID[r.ID] = r
	}
	out := make([]*ToolReturn, 0, len(calls))
	for _, call := range calls {
		if r, ok := byID[call.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// allReturnDirect reports whether every tool referenced in returns is
// registered AND return-direct — the resume-path analog of the
// allReturnDirect bit [toolCallInvoker.invokeToolCalls] computes inline.
func (support *ToolSupport) allReturnDirect(returns []*ToolReturn) bool {
	for _, ret := range returns {
		t, exists := support.registry.Find(ret.Name)
		if !exists || !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// priorModelRounds counts the assistant messages in a resumed conversation
// — the model rounds already spent — so the resumed loop keeps counting
// toward the iteration cap instead of restarting at 1.
func priorModelRounds(msgs []Message) int {
	n := 0
	for _, msg := range msgs {
		if _, ok := msg.(*AssistantMessage); ok {
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

// messagesBeforeAssistant returns the conversation prefix preceding the
// given assistant tool-call message at the tail — stripping a trailing
// partial tool message and the assistant message itself. Used to rebuild
// the round's messages on resume / re-interrupt.
func messagesBeforeAssistant(msgs []Message, assistant *AssistantMessage) []Message {
	end := len(msgs)
	if end > 0 {
		if _, ok := msgs[end-1].(*ToolMessage); ok {
			end--
		}
	}
	if end > 0 {
		if am, ok := msgs[end-1].(*AssistantMessage); ok && am == assistant {
			end--
		}
	}
	return msgs[:end]
}

// wrapInterrupt builds the *ToolLoopInterrupted carrying the resumable
// conversation (prior turns + the assistant tool-call message + the
// results already produced this round) around the tool's interrupt cause.
func (m *ToolMiddleware) wrapInterrupt(prior []Message, assistant *AssistantMessage, done []*ToolReturn, cause error) error {
	return &ToolLoopInterrupted{
		Conversation: interruptedConversation(prior, assistant, done),
		cause:        cause,
	}
}

// resumeCallRound runs the pending tool calls of a resumed round on the
// synchronous path, then re-interrupts, returns direct, or continues the
// loop at the next model round.
func (m *ToolMiddleware) resumeCallRound(ctx context.Context, req *Request, assistant *AssistantMessage, done []*ToolReturn, pending []*ToolCallPart, next CallHandler, support *ToolSupport, state toolLoopState) (*Response, error) {
	res, err := support.invoker.invokeToolCalls(ctx, pending)
	if err != nil {
		return nil, err
	}
	if res.interrupt != nil {
		// Another call in the same round interrupted: fold the results so
		// far into the round's done-set and re-wrap; the assistant turn is
		// unchanged.
		merged := append(slices.Clone(done), res.interrupt.done...)
		return nil, m.wrapInterrupt(messagesBeforeAssistant(req.Messages, assistant), assistant, merged, res.interrupt.cause)
	}

	full := mergeRoundReturns(assistant.CollectToolCalls(), done, res.toolMessage.ToolReturns)
	toolMsg, err := NewToolMessage(full)
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

// resumeStreamRound is the streaming analog of [resumeCallRound]. It
// surfaces the resumed round's tool message to the stream (so the wire
// timeline + caller's per-round budget boundary see it) before continuing.
func (m *ToolMiddleware) resumeStreamRound(ctx context.Context, req *Request, assistant *AssistantMessage, done []*ToolReturn, pending []*ToolCallPart, next StreamHandler, support *ToolSupport, yield func(*Response, error) bool, state toolLoopState) {
	res, err := support.invoker.invokeToolCalls(ctx, pending)
	if err != nil {
		yield(nil, err)
		return
	}
	if res.interrupt != nil {
		merged := append(slices.Clone(done), res.interrupt.done...)
		yield(nil, m.wrapInterrupt(messagesBeforeAssistant(req.Messages, assistant), assistant, merged, res.interrupt.cause))
		return
	}

	full := mergeRoundReturns(assistant.CollectToolCalls(), done, res.toolMessage.ToolReturns)
	toolMsg, err := NewToolMessage(full)
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

// buildResumedContinueRequest assembles the next model request after a
// resumed round completes: the round's prior turns + its assistant
// tool-call message + the assembled tool results, carrying the live
// request's options / tools / params.
func buildResumedContinueRequest(req *Request, assistant *AssistantMessage, toolMsg *ToolMessage) (*Request, error) {
	prior := messagesBeforeAssistant(req.Messages, assistant)
	msgs := append(slices.Clone(prior), Message(assistant), toolMsg)
	next, err := NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}

// buildResumedReturnResponse wraps a resumed round's tool message as the
// final response when every tool in the round is return-direct.
func buildResumedReturnResponse(assistant *AssistantMessage, toolMsg *ToolMessage) (*Response, error) {
	result, err := NewResult(assistant, &ResultMetadata{FinishReason: FinishReasonReturnDirect})
	if err != nil {
		return nil, err
	}
	result.ToolMessage = toolMsg
	return NewResponse(result, &ResponseMetadata{})
}
