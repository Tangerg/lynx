package tool

import (
	stdContext "context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

// InvokeResult represents the result of tool invocation operations in LLM chat interactions.
// It encapsulates the state after tools have been processed and determines the next steps
// in the conversation flow - whether to continue with LLM processing or return results directly.
//
// The result contains:
// - Original chat request and response for context preservation
// - Tool response messages from internal tool executions
// - External tool calls that require client-side processing
// - Flow control flags determining response handling
type InvokeResult struct {
	chatRequest         *chat.Request                 // Original LLM chat request
	chatResponse        *chat.Response                // LLM response containing tool calls
	toolResponseMessage *messages.ToolResponseMessage // Aggregated responses from internal tools
	returnDirect        bool                          // Whether ALL internal tools are configured for direct return
	externalToolCalls   []*messages.ToolCall          // Tool calls requiring external execution
}

// ShouldBuildChatRequest determines if a new chat request should be constructed
// for continued LLM processing. Returns true ONLY when:
// - No external tools exist (external tools always return directly) AND
// - At least one internal tool is configured for LLM integration (returnDirect=false)
//
// External tools always force direct return to client for execution.
// For internal tools: if ANY tool needs LLM processing, the conversation continues with LLM.
// Only when ALL internal tools are configured for direct return does the flow bypass LLM.
func (e *InvokeResult) ShouldBuildChatRequest() bool {
	// External tools always return directly - no LLM continuation
	if len(e.externalToolCalls) > 0 {
		return false
	}
	// For internal tools only: continue with LLM if any tool requires integration
	return !e.returnDirect
}

// ShouldBuildChatResponse determines if a chat response should be constructed
// for direct return to client. Returns true when:
// - External tools exist (always require direct return for client execution) OR
// - ALL internal tools are configured for direct return (returnDirect=true)
//
// This is the inverse of ShouldBuildChatRequest.
func (e *InvokeResult) ShouldBuildChatResponse() bool {
	return !e.ShouldBuildChatRequest()
}

// BuildChatRequest constructs a new chat request for continued LLM processing.
// This method integrates tool responses into the conversation history and
// prepares the request for the next LLM interaction cycle.
//
// This is called ONLY when:
// - No external tools exist (they always return directly)
// - At least one internal tool requires LLM integration (returnDirect=false)
//
// The new request includes:
// - Original conversation history
// - LLM's tool call message
// - Tool response message with execution results
//
// Returns an error if the result is not configured for chat request building
// or if required components are missing.
func (e *InvokeResult) BuildChatRequest() (*chat.Request, error) {
	if !e.ShouldBuildChatRequest() {
		return nil, errors.New("cannot build chat request")
	}
	if e.chatRequest == nil {
		return nil, errors.New("chat request is required")
	}
	if e.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}
	if e.toolResponseMessage == nil {
		return nil, errors.New("tool response message is required")
	}

	chatResult := e.chatResponse.FirstToolCallsResult()
	if chatResult == nil {
		return nil, errors.New("tool calls result is required")
	}

	opts := e.chatRequest.Options().Clone()
	history := e.chatRequest.Instructions()
	msgs := slices.Clone(history)
	msgs = append(msgs, chatResult.Output())
	msgs = append(msgs, e.toolResponseMessage)

	return chat.NewRequest(msgs, opts)
}

// BuildChatResponse constructs a chat response for direct return to client.
// This method creates a response that either:
// - Contains external tool calls for client-side execution (external tools always return directly)
// - Provides direct results when ALL internal tools are configured for direct return
//
// External tools have priority - their presence always forces direct return regardless
// of internal tool configuration.
//
// The response includes:
// - LLM's assistant message with external tool calls (if any)
// - Original response metadata
// - Tool response messages from internal tool executions
//
// Returns an error if the result is not configured for chat response building
// or if required components are missing.
func (e *InvokeResult) BuildChatResponse() (*chat.Response, error) {
	if !e.ShouldBuildChatResponse() {
		return nil, errors.New("cannot build chat response")
	}
	if e.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}

	chatResult := e.chatResponse.FirstToolCallsResult()
	if chatResult == nil {
		return nil, errors.New("tool calls result is required")
	}

	msg := chatResult.Output()
	newMsg := messages.NewAssistantMessage(
		msg.Text(),
		msg.Media(),
		e.externalToolCalls,
		msg.Metadata(),
	)
	newResult, err := chat.NewResult(newMsg, chatResult.Metadata(), e.toolResponseMessage)
	if err != nil {
		return nil, err
	}

	return chat.NewResponse([]*chat.Result{newResult}, e.chatResponse.Metadata())
}

// invokeResultBuilder provides a fluent interface for constructing InvokeResult instances
// with proper validation and configuration. It ensures all required components are
// present before creating the final result.
type invokeResultBuilder struct {
	chatRequest         *chat.Request
	chatResponse        *chat.Response
	toolResponseMessage *messages.ToolResponseMessage
	returnDirect        bool
	externalToolCalls   []*messages.ToolCall
}

// newInvokeResultBuilder creates a new builder for InvokeResult construction.
func newInvokeResultBuilder() *invokeResultBuilder {
	return &invokeResultBuilder{}
}

// withChatRequest sets the original chat request if not nil.
func (e *invokeResultBuilder) withChatRequest(chatRequest *chat.Request) *invokeResultBuilder {
	if chatRequest != nil {
		e.chatRequest = chatRequest
	}
	return e
}

// withChatResponse sets the LLM chat response containing tool calls if not nil.
func (e *invokeResultBuilder) withChatResponse(chatResponse *chat.Response) *invokeResultBuilder {
	if chatResponse != nil {
		e.chatResponse = chatResponse
	}
	return e
}

// withToolResponseMessage sets the aggregated tool response message if not nil.
func (e *invokeResultBuilder) withToolResponseMessage(toolResponseMessage *messages.ToolResponseMessage) *invokeResultBuilder {
	if toolResponseMessage != nil {
		e.toolResponseMessage = toolResponseMessage
	}
	return e
}

// withReturnDirect sets the flag indicating whether results should be returned directly.
func (e *invokeResultBuilder) withReturnDirect(returnDirect bool) *invokeResultBuilder {
	e.returnDirect = returnDirect
	return e
}

// withExternalToolCalls adds external tool calls that require client-side execution.
func (e *invokeResultBuilder) withExternalToolCalls(externalToolCalls ...*messages.ToolCall) *invokeResultBuilder {
	e.externalToolCalls = append(e.externalToolCalls, externalToolCalls...)
	return e
}

// build creates the final InvokeResult instance after validation.
// Returns an error if required components are missing.
func (e *invokeResultBuilder) build() (*InvokeResult, error) {
	if e.chatRequest == nil {
		return nil, errors.New("chat request is required")
	}
	if e.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}
	if e.toolResponseMessage == nil && len(e.externalToolCalls) == 0 {
		return nil, errors.New("tool response or external tool calls is required")
	}
	return &InvokeResult{
		chatRequest:         e.chatRequest,
		chatResponse:        e.chatResponse,
		toolResponseMessage: e.toolResponseMessage,
		returnDirect:        e.returnDirect,
		externalToolCalls:   e.externalToolCalls,
	}, nil
}

// invoker handles the execution of tool calls from LLM responses.
// It processes both internal tools (executed immediately) and external tools
// (delegated to client), managing the flow control and result aggregation.
type invoker struct {
	registry *Registry // Tool registry for looking up available tools
}

// newInvoker creates a new tool invoker with the specified registry.
func newInvoker(registry *Registry) *invoker {
	return &invoker{
		registry: registry,
	}
}

// shouldInvokeToolCalls determines if the chat response contains valid tool calls
// that can be processed. It validates that all requested tools exist in the registry.
//
// Returns true if tool calls should be processed, false otherwise.
// Returns an error if any tool call references an unregistered tool.
func (e *invoker) shouldInvokeToolCalls(chatResponse *chat.Response) (bool, error) {
	chatResult := chatResponse.FirstToolCallsResult()
	if chatResult == nil {
		return false, nil
	}

	for _, toolCall := range chatResult.Output().ToolCalls() {
		_, ok := e.registry.Find(toolCall.Name)
		if !ok {
			return false, fmt.Errorf("invalid tool call name: %s", toolCall.Name)
		}
		return true, nil
	}
	return false, nil
}

// makeContext creates a new execution context for tool operations.
func (e *invoker) makeContext(stdCtx stdContext.Context, chatRequest *chat.Request) Context {
	ctx := NewContext(stdCtx)
	toolOptions, ok := chatRequest.Options().(Options)
	if ok {
		return ctx.SetMap(toolOptions.ToolParams())
	}
	return ctx
}

// invokeToolCalls processes a list of tool calls, executing internal tools immediately
// and collecting external tools for client-side processing.
//
// The method:
// - Separates internal tools (CallableTool) from external tools
// - Executes internal tools and collects their responses
// - Determines overall flow control based on tool metadata
// - Returns a builder configured with the execution results
func (e *invoker) invokeToolCalls(ctx Context, toolCalls []*messages.ToolCall) (*invokeResultBuilder, error) {
	var (
		externalToolCalls []*messages.ToolCall     // Tools requiring external execution
		returnDirect      = true                   // Whether to return results directly
		toolCallResponses []*messages.ToolResponse // Responses from internal tools
	)

	for _, toolCall := range toolCalls {
		// Tool existence guaranteed by shouldInvokeToolCalls precheck
		t, _ := e.registry.Find(toolCall.Name)
		ct, ok := t.(CallableTool)
		if !ok {
			// External tool - add to delegation list
			externalToolCalls = append(externalToolCalls, toolCall)
			continue
		}

		// Internal tool - execute immediately
		resp, err := ct.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to invoke tool call %s", toolCall.Name))
		}

		// Update flow control based on tool metadata
		returnDirect = returnDirect && ct.Metadata().ReturnDirect()
		toolCallResponses = append(toolCallResponses, &messages.ToolResponse{
			ID:           toolCall.ID,
			Name:         toolCall.Name,
			ResponseData: resp,
		})
	}

	// Create tool response message from internal tool results
	toolMessage, err := messages.NewToolResponseMessage(toolCallResponses, nil)
	if err != nil {
		return nil, err
	}

	return newInvokeResultBuilder().
		withToolResponseMessage(toolMessage).
		withExternalToolCalls(externalToolCalls...).
		withReturnDirect(returnDirect), nil
}

// invoke orchestrates the complete tool invocation process for an LLM chat interaction.
// It validates tool calls, executes available tools, and constructs the appropriate
// result for the next step in the conversation flow.
//
// The process:
// 1. Validates that tool calls should be processed
// 2. Executes internal tools and collects external tools
// 3. Constructs InvokeResult with proper flow control configuration
//
// Returns InvokeResult for determining next conversation steps, or error if processing fails.
func (e *invoker) invoke(ctx stdContext.Context, chatRequest *chat.Request, chatResponse *chat.Response) (*InvokeResult, error) {
	shouldInvokeToolCalls, err := e.shouldInvokeToolCalls(chatResponse)
	if err != nil {
		return nil, err
	}
	if !shouldInvokeToolCalls {
		return nil, errors.New("cannot invoke tool calls")
	}

	// Tool calls result guaranteed by shouldInvokeToolCalls precheck
	chatResult := chatResponse.FirstToolCallsResult()

	executionResultBuilder, err := e.invokeToolCalls(
		e.makeContext(ctx, chatRequest),
		chatResult.Output().ToolCalls(),
	)
	if err != nil {
		return nil, err
	}

	return executionResultBuilder.
		withChatRequest(chatRequest).
		withChatResponse(chatResponse).
		build()
}
