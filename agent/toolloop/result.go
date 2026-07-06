package toolloop

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// invocationResult is what the tool-calling middleware emits after
// running the LLM-requested tool calls. It captures the inline results
// (toolMessage) plus the flow-control bit (allReturnDirect) that decides
// whether to feed results back to the LLM or return them to the caller.
type invocationResult struct {
	request         *chat.Request
	response        *chat.Response
	toolMessage     *chat.ToolMessage
	allReturnDirect bool

	// interrupt is set when a tool call halted the round for human input
	// (HITL). It carries the results produced so far this round plus the
	// interrupt cause; the middleware turns it into a FinishReasonInterrupt
	// response (the resumable tail) and propagates the cause so the caller
	// parks. When set, toolMessage is nil.
	interrupt *roundInterrupt
}

// roundInterrupt is the partial result of a tool round that halted for human
// input: the results already produced this invocation (done, in call order)
// and the interrupt cause (a [Halt]). The still-pending calls are
// derived from the assistant message at resume time, so they are not carried
// here.
type roundInterrupt struct {
	done  []*chat.ToolReturn
	cause error
}

// shouldContinue reports whether the runtime should re-prompt the LLM
// with the tool results. It is true when at least one internal tool
// wants its result fed back to the LLM.
func (r *invocationResult) shouldContinue() bool {
	return !r.allReturnDirect
}

// shouldReturn is the inverse of [invocationResult.shouldContinue].
func (r *invocationResult) shouldReturn() bool { return !r.shouldContinue() }

// buildContinueRequest assembles the next request when the round wants an LLM
// follow-up. It validates the continue state, then defers the message assembly
// to [nextRoundRequest].
func (r *invocationResult) buildContinueRequest() (*chat.Request, error) {
	if !r.shouldContinue() {
		return nil, errors.New("toolloop.invocationResult.buildContinueRequest: result is in return-direct state")
	}
	if err := r.validate(); err != nil {
		return nil, err
	}

	result := r.response.Result
	if result == nil || !result.AssistantMessage.HasToolCalls() {
		return nil, errors.New("toolloop.invocationResult.buildContinueRequest: response has no tool calls")
	}
	return nextRoundRequest(r.request, result.AssistantMessage, r.toolMessage)
}

// buildReturnResponse assembles the final [*chat.Response] when no further
// LLM round is needed — every internal tool was return-direct.
func (r *invocationResult) buildReturnResponse() (*chat.Response, error) {
	if !r.shouldReturn() {
		return nil, errors.New("toolloop.invocationResult.buildReturnResponse: result is in continue state")
	}
	if r.response == nil {
		return nil, errors.New("toolloop.invocationResult.buildReturnResponse: LLM response is missing")
	}

	withCalls := r.response.Result
	if withCalls == nil || !withCalls.AssistantMessage.HasToolCalls() {
		return nil, errors.New("toolloop.invocationResult.buildReturnResponse: response has no tool calls")
	}

	result, err := chat.NewResult(withCalls.AssistantMessage, withCalls.Metadata)
	if err != nil {
		return nil, fmt.Errorf("toolloop.invocationResult.buildReturnResponse: %w", err)
	}
	result.ToolMessage = r.toolMessage

	return chat.NewResponse(result, r.response.Metadata)
}

func (r *invocationResult) validate() error {
	if r.request == nil {
		return errors.New("toolloop.invocationResult: original request is missing")
	}
	if r.response == nil {
		return errors.New("toolloop.invocationResult: LLM response is missing")
	}
	if r.toolMessage == nil {
		return errors.New("toolloop.invocationResult: internal-tools message is required")
	}
	return nil
}

// shouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*chat.ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool is marked with [ReturnDirect].
func (i *invoker) shouldReturnDirect(msgs []chat.Message) bool {
	last, ok := pkgSlices.Last(msgs)
	if !ok {
		return false
	}
	toolMsg, ok := last.(*chat.ToolMessage)
	if !ok {
		return false
	}

	for _, ret := range toolMsg.ToolReturns {
		t, exists := i.registry.find(ret.Name)
		if !exists {
			return false
		}
		if !returnsDirect(t) {
			return false
		}
	}
	return true
}

// buildReturnDirectResponse assembles a synthetic [*chat.Response] that wraps
// the last [*chat.ToolMessage] as the final answer. Returns an error when
// [invoker.shouldReturnDirect] would return false.
func (i *invoker) buildReturnDirectResponse(msgs []chat.Message) (*chat.Response, error) {
	if !i.shouldReturnDirect(msgs) {
		return nil, errors.New("toolloop.invoker.buildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := chat.NewAssistantMessage(map[string]any{
		"created_by": FinishReasonReturnDirect.String(),
	})
	// shouldReturnDirect already verified the tail is a *chat.ToolMessage.
	return toolRoundResponse(assistantMsg, last.(*chat.ToolMessage), FinishReasonReturnDirect)
}

// systemMessages returns the system messages of msgs (zero or one in
// practice). The tool loop forwards them on every downstream request so the
// model always sees the turn's system header first; the history middleware
// never stores system messages, so they ride along with each round.
func systemMessages(msgs []chat.Message) []chat.Message {
	return chat.MessageList(msgs).FilterTypes(chat.MessageTypeSystem)
}

// nextRoundRequest assembles the next model request from the turn's system
// header plus this round's (assistant tool-call, tool result) exchange,
// carrying the live request's options / tools / params. Shared by the normal
// loop ([invocationResult.buildContinueRequest]) and HITL resume
// ([middleware.resumeCall] / [middleware.resumeStream]).
//
// It deliberately does NOT re-send the prior conversation — the history
// middleware below the loop owns the stored history and splices it back in. The
// assistant tool-call message DOES travel alongside its tool result so the two
// persist as one atomic exchange (history skips a lone tool-call assistant, so
// it can never strand an unanswered assistant(tool_calls) if the turn
// interrupts mid-round). Re-sending the full conversation, by contrast, is the
// coupling that forced the history layer to de-duplicate.
func nextRoundRequest(req *chat.Request, assistant *chat.AssistantMessage, toolMsg *chat.ToolMessage) (*chat.Request, error) {
	msgs := append(systemMessages(req.Messages), assistant, toolMsg)
	next, err := chat.NewRequest(msgs)
	if err != nil {
		return nil, err
	}
	next.Options = req.Options.Clone()
	next.Tools = slices.Clone(req.Tools)
	next.Params = maps.Clone(req.Params)
	return next, nil
}
