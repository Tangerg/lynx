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
// capabilities, offering a simplified API for common tool-related operations.
//
// The Helper orchestrates the complete tool processing workflow:
// - Managing immutable tool registrations through an internal registry
// - Validating and executing tool calls from LLM responses
// - Determining conversation flow control based on tool configurations
// - Handling both internal tools (executed immediately) and external tools (delegated to client)
type Helper struct {
	registry *Registry // Thread-safe registry for managing immutable tool instances
	invoker  *invoker  // Internal tool invocation processor
}

// NewHelper creates a new Helper instance with an internal tool registry.
// The optional cap parameter specifies the initial capacity for the tool registry.
// If no capacity is provided or a negative value is given, it defaults to 0.
//
// The Helper provides a unified interface for tool management and processing,
// eliminating the need to manage registry and invoker separately.
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
// This provides access to the underlying registry for advanced tool registration,
// lookup, and management operations.
//
// The returned registry is thread-safe and shares the same tool instances
// used by the Helper's invocation operations.
func (m *Helper) Registry() *Registry {
	return m.registry
}

// RegisterTools fast to use
func (m *Helper) RegisterTools(tools ...Tool) {
	m.registry.Register(tools...)
}

// ShouldReturnDirect determines if a conversation should return directly to the user
// based on the last message in the conversation history. This method analyzes tool
// response messages to determine if ALL tools involved are configured for direct return.
//
// Returns true when:
// - The last message is a ToolResponseMessage AND
// - ALL tools referenced in the message are registered AND
// - ALL tools are configured with returnDirect=true
//
// This method is typically used to determine conversation flow after tool execution,
// helping decide whether to continue LLM processing or return results directly.
//
// Note: If any tool is missing from the registry, returns false for safety.
func (m *Helper) ShouldReturnDirect(msgs []messages.Message) bool {
	// Check if the last message is a tool response
	if !messages.HasTypeAtLast(msgs, messages.Tool) {
		return false
	}

	message, _ := pkgSlices.Last(msgs)
	toolResponseMessage, ok := message.(*messages.ToolResponseMessage)
	if !ok {
		return false
	}

	var (
		returnDirect = true
		t            Tool
	)
	for _, toolResponse := range toolResponseMessage.ToolResponses() {
		// Verify tool exists in registry
		t, ok = m.registry.Find(toolResponse.Name)
		if !ok {
			return false // Unknown tool - cannot determine behavior
		}
		// ALL tools must be configured for direct return
		returnDirect = returnDirect && t.Metadata().ReturnDirect()
	}

	return returnDirect
}

func (m *Helper) MakeReturnDirectChatResponse(msgs []messages.Message) (*chat.Response, error) {
	if !m.ShouldReturnDirect(msgs) {
		return nil, errors.New("cannot build chat response")
	}
	message, _ := pkgSlices.Last(msgs)
	// prechecked by ShouldReturnDirect
	toolResponseMessage := message.(*messages.ToolResponseMessage)

	assistantMessage := messages.NewAssistantMessage(map[string]any{
		"create_by": chat.ReturnDirect.String(),
	})
	metadata := &chat.ResultMetadata{
		FinishReason: chat.ReturnDirect,
	}
	chatResult, err := chat.NewResult(assistantMessage, metadata, toolResponseMessage)
	if err != nil {
		return nil, err
	}
	return chat.NewResponse([]*chat.Result{chatResult}, &chat.ResponseMetadata{})
}

// ShouldInvokeToolCalls determines if the chat response contains valid tool calls
// that should be processed. This method validates that all requested tools exist
// in the registry and are available for invocation.
//
// Returns true if tool calls should be processed, false otherwise.
// Returns an error if any tool call references an unregistered tool.
//
// This is typically the first step in tool processing workflow, used to validate
// LLM responses before attempting tool execution.
func (m *Helper) ShouldInvokeToolCalls(chatResponse *chat.Response) (bool, error) {
	return m.invoker.shouldInvokeToolCalls(chatResponse)
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
// - External tools always force direct return (ignore returnDirect settings)
// - Internal tools: if ANY tool has returnDirect=false, continue with LLM processing
// - Only when ALL internal tools have returnDirect=true does the flow bypass LLM
//
// Returns InvokeResult containing:
// - Original chat context for flow control
// - Tool response messages from internal executions
// - External tool calls for client-side processing
// - Flow control flags for determining next conversation steps
//
// Returns an error if tool validation fails or tool execution encounters errors.
func (m *Helper) InvokeToolCalls(ctx stdContext.Context, chatRequest *chat.Request, chatResponse *chat.Response) (*InvokeResult, error) {
	return m.invoker.invoke(ctx, chatRequest, chatResponse)
}
