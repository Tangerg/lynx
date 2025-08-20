package tool

import (
	stdContext "context"
	"errors"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// Helper provides a high-level interface for managing tools and processing tool calls
// in LLM chat interactions. It combines tool registry management with tool invocation
// capabilities for common tool-related operations.
type Helper struct {
	registry *Registry // tool registry for managing tool instances
	invoker  *invoker  // tool invocation processor
}

// NewHelper creates a new Helper instance with an internal tool registry.
// The optional cap parameter specifies the initial capacity for the tool registry.
//
// Example:
//
//	helper := NewHelper()       // Default capacity
//	helper := NewHelper(50)     // Initial capacity of 50 tools
func NewHelper(cap ...int) *Helper {
	registry := NewRegistry(cap...)
	return &Helper{
		registry: registry,
		invoker:  newInvoker(registry),
	}
}

// Registry returns the internal tool registry for direct tool management operations.
func (h *Helper) Registry() *Registry {
	return h.registry
}

// RegisterTools registers multiple tools to the internal registry.
func (h *Helper) RegisterTools(tools ...Tool) {
	h.registry.Register(tools...)
}

// ShouldReturnDirect determines if a conversation should return directly to the user
// based on the last message in the conversation history.
//
// Returns true when:
// - The last message is a ToolMessage AND
// - ALL tools referenced in the message are registered AND
// - ALL tools are configured with returnDirect=true
func (h *Helper) ShouldReturnDirect(msgs []messages.Message) bool {
	// Check if the last message is a tool response
	if !messages.HasTypeAtLast(msgs, messages.Tool) {
		return false
	}

	message, _ := pkgSlices.Last(msgs)
	toolResponseMessage, ok := message.(*messages.ToolMessage)
	if !ok {
		return false
	}

	returnDirect := true
	for _, toolResponse := range toolResponseMessage.ToolReturns() {
		// Verify tool exists in registry
		t, ok1 := h.registry.Find(toolResponse.Name)
		if !ok1 {
			return false // Unknown tool - cannot determine behavior
		}

		// ALL tools must be configured for direct return
		returnDirect = returnDirect && t.Metadata().ReturnDirect
	}

	return returnDirect
}

// MakeReturnDirectChatResponse creates a chat response for direct return when all tools
// are configured for direct return.
func (h *Helper) MakeReturnDirectChatResponse(msgs []messages.Message) (*chat.Response, error) {
	if !h.ShouldReturnDirect(msgs) {
		return nil, errors.New("cannot build chat response")
	}

	message, _ := pkgSlices.Last(msgs)
	// prechecked by ShouldReturnDirect
	toolResponseMessage := message.(*messages.ToolMessage)

	assistantMessage := messages.NewAssistantMessage(map[string]any{
		"create_by": chat.FinishReasonReturnDirect.String(),
	})

	metadata := &chat.ResultMetadata{
		FinishReason: chat.FinishReasonReturnDirect,
	}

	result, err := chat.NewResult(assistantMessage, metadata, toolResponseMessage)
	if err != nil {
		return nil, err
	}

	return chat.NewResponse([]*chat.Result{result}, &chat.ResponseMetadata{})
}

// ShouldInvokeToolCalls determines if the chat response contains valid tool calls
// that should be processed. It validates that all requested tools exist in the registry.
func (h *Helper) ShouldInvokeToolCalls(response *chat.Response) (bool, error) {
	return h.invoker.shouldInvokeToolCalls(response)
}

// InvokeToolCalls processes tool calls from an LLM chat response, executing internal tools
// immediately and preparing external tools for client-side execution.
//
// The method orchestrates the complete tool invocation workflow:
// 1. Validates that tool calls should be processed
// 2. Separates internal tools (CallableTool) from external tools
// 3. Executes internal tools and collects their responses
// 4. Determines conversation flow control based on tool metadata and external tool presence
// 5. Constructs InvokeResult with appropriate configuration for next steps
//
// Flow control logic:
// - External tools always force direct return
// - Internal tools: if ANY tool has returnDirect=false, continue with LLM processing
// - Only when ALL internal tools have returnDirect=true does the flow bypass LLM
func (h *Helper) InvokeToolCalls(ctx stdContext.Context, request *chat.Request, response *chat.Response) (*InvokeResult, error) {
	return h.invoker.invoke(ctx, request, response)
}
