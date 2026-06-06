package tool

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// Support bundles a [Registry] with a tool-call invoker — the
// integration point the [NewMiddleware] middleware uses to drive
// the tool-calling loop. See [NewMiddleware] for end-to-end wiring.
type Support struct {
	registry *Registry
	invoker  *callInvoker
}

// NewSupport returns a [Support] backed by a fresh registry.
// capacityHint, if positive, preallocates the registry's backing map.
func NewSupport(capacityHint ...int) *Support {
	registry := newRegistry(capacityHint...)
	return &Support{
		registry: registry,
		invoker:  newCallInvoker(registry),
	}
}

// Registry exposes the underlying [Registry] for direct access.
func (s *Support) Registry() *Registry { return s.registry }

// Register is a shorthand for [Registry.Register].
func (s *Support) Register(tools ...chat.Tool) {
	s.registry.Register(tools...)
}

// Unregister is a shorthand for [Registry.Unregister].
func (s *Support) Unregister(names ...string) {
	s.registry.Unregister(names...)
}

// ShouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*chat.ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool has ReturnDirect = true.
func (s *Support) ShouldReturnDirect(msgs []chat.Message) bool {
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
		t, exists := s.registry.Find(ret.Name)
		if !exists {
			return false
		}
		if !t.Metadata().ReturnDirect {
			return false
		}
	}
	return true
}

// BuildReturnDirectResponse assembles a synthetic [*chat.Response] that wraps
// the last [*chat.ToolMessage] as the final answer. Returns an error when
// [Support.ShouldReturnDirect] would return false.
func (s *Support) BuildReturnDirectResponse(msgs []chat.Message) (*chat.Response, error) {
	if !s.ShouldReturnDirect(msgs) {
		return nil, errors.New("tool.Support.BuildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := chat.NewAssistantMessage(map[string]any{
		"created_by": chat.FinishReasonReturnDirect.String(),
	})
	metadata := &chat.ResultMetadata{FinishReason: chat.FinishReasonReturnDirect}

	result, err := chat.NewResult(assistantMsg, metadata)
	if err != nil {
		return nil, fmt.Errorf("tool.Support.BuildReturnDirectResponse: %w", err)
	}

	// ShouldReturnDirect already verified this is a *chat.ToolMessage.
	result.ToolMessage = last.(*chat.ToolMessage)

	return chat.NewResponse(result, &chat.ResponseMetadata{})
}

// ShouldInvokeToolCalls reports whether the response contains tool
// calls that the registry can fulfill.
func (s *Support) ShouldInvokeToolCalls(resp *chat.Response) (bool, error) {
	return s.invoker.canInvokeToolCalls(resp)
}

// InvokeToolCalls runs one tool-calling round: validate, dispatch every
// tool, and assemble the result.
//
// Flow control: when every invoked tool is return-direct the loop ends;
// otherwise the caller is expected to re-prompt the LLM via
// [InvocationResult.BuildContinueRequest].
func (s *Support) InvokeToolCalls(ctx context.Context, req *chat.Request, resp *chat.Response) (*InvocationResult, error) {
	return s.invoker.invoke(ctx, req, resp)
}
