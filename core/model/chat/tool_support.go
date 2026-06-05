package chat

import (
	"context"
	"errors"
	"fmt"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ToolSupport bundles a [ToolRegistry] with a tool-call invoker — the
// integration point the [NewToolMiddleware] middleware uses to drive
// the tool-calling loop. See [NewToolMiddleware] for end-to-end wiring.
type ToolSupport struct {
	registry *ToolRegistry
	invoker  *toolCallInvoker
}

// NewToolSupport returns a [ToolSupport] backed by a fresh registry.
// capacityHint, if positive, preallocates the registry's backing map.
func NewToolSupport(capacityHint ...int) *ToolSupport {
	registry := newToolRegistry(capacityHint...)
	return &ToolSupport{
		registry: registry,
		invoker:  newToolCallInvoker(registry),
	}
}

// Registry exposes the underlying [ToolRegistry] for direct access.
func (s *ToolSupport) Registry() *ToolRegistry { return s.registry }

// SetFeedbackOnUnknownTool toggles unknown-tool tolerance. When enabled, a
// call to an unregistered tool yields an error result fed back to the model
// (so it can pick a real tool) instead of aborting the whole request. The
// default is off, preserving the strict "unknown tool is an error" behavior.
func (s *ToolSupport) SetFeedbackOnUnknownTool(enabled bool) {
	s.invoker.feedbackOnUnknown = enabled
}

// SetFeedbackOnToolError toggles tool-execution-error tolerance. When
// enabled, a tool whose Call returns an error yields an error result fed
// back to the model (so it can adjust and continue) instead of aborting the
// whole request. The default is off, preserving the strict "tool error
// aborts" behavior.
func (s *ToolSupport) SetFeedbackOnToolError(enabled bool) {
	s.invoker.feedbackOnError = enabled
}

// Register is a shorthand for [ToolRegistry.Register].
func (s *ToolSupport) Register(tools ...Tool) {
	s.registry.Register(tools...)
}

// Unregister is a shorthand for [ToolRegistry.Unregister].
func (s *ToolSupport) Unregister(names ...string) {
	s.registry.Unregister(names...)
}

// ShouldReturnDirect reports whether the conversation should end with
// the most recent tool message (no further LLM round). It is true only
// when:
//   - the last message is a [*ToolMessage], AND
//   - every tool referenced in that message is registered, AND
//   - every such tool has ReturnDirect = true.
func (s *ToolSupport) ShouldReturnDirect(msgs []Message) bool {
	if !hasMessageTypeAtLast(msgs, MessageTypeTool) {
		return false
	}

	last, _ := pkgSlices.Last(msgs)
	toolMsg, ok := last.(*ToolMessage)
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

// BuildReturnDirectResponse assembles a synthetic [*Response] that wraps
// the last [*ToolMessage] as the final answer. Returns an error when
// [ToolSupport.ShouldReturnDirect] would return false.
func (s *ToolSupport) BuildReturnDirectResponse(msgs []Message) (*Response, error) {
	if !s.ShouldReturnDirect(msgs) {
		return nil, errors.New("chat.ToolSupport.BuildReturnDirectResponse: conditions for return-direct are not met")
	}
	last, _ := pkgSlices.Last(msgs)

	assistantMsg := NewAssistantMessage(map[string]any{
		"created_by": FinishReasonReturnDirect.String(),
	})
	metadata := &ResultMetadata{FinishReason: FinishReasonReturnDirect}

	result, err := NewResult(assistantMsg, metadata)
	if err != nil {
		return nil, fmt.Errorf("chat.ToolSupport.BuildReturnDirectResponse: %w", err)
	}

	// ShouldReturnDirect already verified this is a *ToolMessage.
	result.ToolMessage = last.(*ToolMessage)

	return NewResponse(result, &ResponseMetadata{})
}

// ShouldInvokeToolCalls reports whether the response contains tool
// calls that the registry can fulfill.
func (s *ToolSupport) ShouldInvokeToolCalls(resp *Response) (bool, error) {
	return s.invoker.canInvokeToolCalls(resp)
}

// InvokeToolCalls runs one tool-calling round: validate, dispatch every
// tool, and assemble the result.
//
// Flow control: when every invoked tool is return-direct the loop ends;
// otherwise the caller is expected to re-prompt the LLM via
// [ToolInvocationResult.BuildContinueRequest].
func (s *ToolSupport) InvokeToolCalls(ctx context.Context, req *Request, resp *Response) (*ToolInvocationResult, error) {
	return s.invoker.invoke(ctx, req, resp)
}
