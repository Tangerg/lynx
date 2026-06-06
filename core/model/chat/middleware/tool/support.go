package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// support bundles a [registry] with a tool-call invoker — the
// integration point the [NewMiddleware] middleware uses to drive
// the tool-calling loop. See [NewMiddleware] for end-to-end wiring.
type support struct {
	registry *registry
	invoker  *callInvoker
}

// newSupport returns a [support] backed by a fresh registry.
// capacityHint, if positive, preallocates the registry's backing map.
func newSupport(capacityHint ...int) *support {
	registry := newRegistry(capacityHint...)
	return &support{
		registry: registry,
		invoker:  newCallInvoker(registry),
	}
}

// register is a shorthand for [registry.register].
func (s *support) register(tools ...chat.Tool) {
	s.registry.register(tools...)
}

// unregister is a shorthand for [registry.unregister].
func (s *support) unregister(names ...string) {
	s.registry.unregister(names...)
}

// shouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*chat.ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool has ReturnDirect = true.
func (s *support) shouldReturnDirect(msgs []chat.Message) bool {
	if len(msgs) == 0 {
		return false
	}
	if _, ok := msgs[len(msgs)-1].(*chat.ToolMessage); !ok {
		return false
	}

	last, _ := pkgSlices.Last(msgs)
	toolMsg, ok := last.(*chat.ToolMessage)
	if !ok {
		return false
	}

	for _, ret := range toolMsg.ToolReturns {
		t, exists := s.registry.find(ret.Name)
		if !exists {
			return false
		}
		if !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// buildReturnDirectResponse assembles a synthetic [*chat.Response] that wraps
// the last [*chat.ToolMessage] as the final answer. Returns an error when
// [support.shouldReturnDirect] would return false.
func (s *support) buildReturnDirectResponse(msgs []chat.Message) (*chat.Response, error) {
	if !s.shouldReturnDirect(msgs) {
		return nil, errors.New("tool.support.buildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := chat.NewAssistantMessage(map[string]any{
		"created_by": chat.FinishReasonReturnDirect.String(),
	})
	metadata := &chat.ResultMetadata{FinishReason: chat.FinishReasonReturnDirect}

	result, err := chat.NewResult(assistantMsg, metadata)
	if err != nil {
		return nil, fmt.Errorf("tool.support.buildReturnDirectResponse: %w", err)
	}

	// ShouldReturnDirect already verified this is a *chat.ToolMessage.
	result.ToolMessage = last.(*chat.ToolMessage)

	return chat.NewResponse(result, &chat.ResponseMetadata{})
}

// shouldInvokeToolCalls reports whether the response contains tool
// calls that the registry can fulfill.
func (s *support) shouldInvokeToolCalls(resp *chat.Response) (bool, error) {
	return s.invoker.canInvokeToolCalls(resp)
}

// invokeToolCalls runs one tool-calling round: validate, dispatch every
// tool, and assemble the result.
//
// Flow control: when every invoked tool is return-direct the loop ends;
// otherwise the caller is expected to re-prompt the LLM via
// [invocationResult.buildContinueRequest].
func (s *support) invokeToolCalls(ctx context.Context, req *chat.Request, resp *chat.Response) (*invocationResult, error) {
	return s.invoker.invoke(ctx, req, resp)
}
