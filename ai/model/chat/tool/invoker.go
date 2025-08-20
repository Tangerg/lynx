package tool

import (
	stdContext "context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat"
	"github.com/Tangerg/lynx/ai/model/chat/messages"
)

// InvokeResult represents the outcome of tool invocation in LLM chat interactions.
// It encapsulates execution results and determines the next steps in conversation flow.
type InvokeResult struct {
	chatRequest         *chat.Request         // original LLM chat request
	chatResponse        *chat.Response        // LLM response containing tool calls
	toolResponseMessage *messages.ToolMessage // aggregated responses from internal tools
	returnDirect        bool                  // whether ALL internal tools are configured for direct return
	externalToolCalls   []*messages.ToolCall  // tool calls requiring external execution
}

// ShouldMakeChatRequest determines if a new chat request should be constructed
// for continued LLM processing. Returns true ONLY when:
// - No external tools exist (external tools always return directly) AND
// - At least one internal tool is configured for LLM integration (returnDirect=false)
func (r *InvokeResult) ShouldMakeChatRequest() bool {
	// External tools always return directly - no LLM continuation
	if len(r.externalToolCalls) > 0 {
		return false
	}

	// For internal tools only: continue with LLM if any tool requires integration
	return !r.returnDirect
}

// ShouldMakeChatResponse determines if a chat response should be constructed
// for direct return to client. This is the inverse of ShouldMakeChatRequest.
func (r *InvokeResult) ShouldMakeChatResponse() bool {
	return !r.ShouldMakeChatRequest()
}

// MakeChatRequest constructs a new chat request for continued LLM processing.
// This integrates tool responses into conversation history and prepares the request
// for the next LLM interaction cycle.
//
// Called ONLY when no external tools exist and at least one internal tool
// requires LLM integration (returnDirect=false).
func (r *InvokeResult) MakeChatRequest() (*chat.Request, error) {
	if !r.ShouldMakeChatRequest() {
		return nil, errors.New("cannot build chat request")
	}
	if r.chatRequest == nil {
		return nil, errors.New("chat request is required")
	}
	if r.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}
	if r.toolResponseMessage == nil {
		return nil, errors.New("tool response message is required")
	}

	result := r.chatResponse.FirstToolCallsResult()
	if result == nil {
		return nil, errors.New("tool calls result is required")
	}

	opts := r.chatRequest.Options().Clone()
	history := r.chatRequest.Instructions()
	msgs := slices.Clone(history)
	msgs = append(msgs, result.Output())
	msgs = append(msgs, r.toolResponseMessage)

	return chat.NewRequest(msgs, opts)
}

// MakeChatResponse constructs a chat response for direct return to client.
// This creates a response that either contains external tool calls for client-side
// execution or provides direct results when ALL internal tools are configured
// for direct return.
func (r *InvokeResult) MakeChatResponse() (*chat.Response, error) {
	if !r.ShouldMakeChatResponse() {
		return nil, errors.New("cannot build chat response")
	}
	if r.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}

	result := r.chatResponse.FirstToolCallsResult()
	if result == nil {
		return nil, errors.New("tool calls result is required")
	}

	msg := result.Output()
	newMsg := messages.NewAssistantMessage(
		messages.MessageParams{
			Text:      msg.Text(),
			Media:     msg.Media(),
			ToolCalls: r.externalToolCalls,
			Metadata:  msg.Metadata(),
		})

	newResult, err := chat.NewResult(newMsg, result.Metadata(), r.toolResponseMessage)
	if err != nil {
		return nil, err
	}

	return chat.NewResponse([]*chat.Result{newResult}, r.chatResponse.Metadata())
}

func validInvokeResult(result *InvokeResult) error {
	if result.chatRequest == nil {
		return errors.New("chat request is required")
	}
	if result.chatResponse == nil {
		return errors.New("chat response is required")
	}
	if result.toolResponseMessage == nil && len(result.externalToolCalls) == 0 {
		return errors.New("tool response or external tool calls is required")
	}
	return nil
}

// invoker handles the execution of tool calls from LLM responses.
// It processes both internal tools (executed immediately) and external tools
// (delegated to client), managing flow control and result aggregation.
type invoker struct {
	registry *Registry // tool registry for looking up available tools
}

// newInvoker creates a new tool invoker with the specified registry.
func newInvoker(registry *Registry) *invoker {
	return &invoker{
		registry: registry,
	}
}

// shouldInvokeToolCalls determines if the chat response contains valid tool calls
// that can be processed. It validates that all requested tools exist in the registry.
func (i *invoker) shouldInvokeToolCalls(response *chat.Response) (bool, error) {
	result := response.FirstToolCallsResult()
	if result == nil {
		return false, nil
	}

	for _, toolCall := range result.Output().ToolCalls() {
		_, ok := i.registry.Find(toolCall.Name)
		if !ok {
			return false, fmt.Errorf("invalid tool call name: %s", toolCall.Name)
		}
		return true, nil
	}

	return false, nil
}

// makeContext creates a new execution context for tool operations.
func (i *invoker) makeContext(stdCtx stdContext.Context, request *chat.Request) *Context {
	ctx := NewContext(stdCtx)

	if toolOptions, ok := request.Options().(Options); ok {
		return ctx.SetMap(toolOptions.ToolParams())
	}

	return ctx
}

// invokeToolCalls processes a list of tool calls, executing internal tools immediately
// and collecting external tools for client-side processing.
func (i *invoker) invokeToolCalls(ctx *Context, toolCalls []*messages.ToolCall) (*InvokeResult, error) {
	var (
		externalToolCalls []*messages.ToolCall   // tools requiring external execution
		returnDirect      = true                 // whether to return results directly
		responses         []*messages.ToolReturn // responses from internal tools
	)

	for _, toolCall := range toolCalls {
		// Tool existence guaranteed by shouldInvokeToolCalls precheck
		t, _ := i.registry.Find(toolCall.Name)

		ct, ok := t.(CallableTool)
		if !ok {
			// External tool - add to delegation list
			externalToolCalls = append(externalToolCalls, toolCall)
			continue
		}

		// Internal tool - execute immediately
		resp, err := ct.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to invoke tool call %s. %w", toolCall.Name, err)
		}

		// Update flow control based on tool metadata
		returnDirect = returnDirect && ct.Metadata().ReturnDirect
		responses = append(responses, &messages.ToolReturn{
			ID:     toolCall.ID,
			Name:   toolCall.Name,
			Result: resp,
		})
	}

	// Create tool response message from internal tool results
	toolResponseMessage, err := messages.NewToolMessage(responses)
	if err != nil {
		return nil, err
	}

	return &InvokeResult{
		toolResponseMessage: toolResponseMessage,
		externalToolCalls:   externalToolCalls,
		returnDirect:        returnDirect,
	}, nil
}

// invoke orchestrates the complete tool invocation process for an LLM chat interaction.
// It validates tool calls, executes available tools, and constructs the appropriate
// result for the next step in the conversation flow.
func (i *invoker) invoke(ctx stdContext.Context, request *chat.Request, response *chat.Response) (*InvokeResult, error) {
	shouldInvoke, err := i.shouldInvokeToolCalls(response)
	if err != nil {
		return nil, err
	}
	if !shouldInvoke {
		return nil, errors.New("cannot invoke tool calls")
	}

	// Tool calls result guaranteed by shouldInvokeToolCalls precheck
	result := response.FirstToolCallsResult()

	invokeResult, err := i.invokeToolCalls(
		i.makeContext(ctx, request),
		result.Output().ToolCalls(),
	)
	if err != nil {
		return nil, err
	}

	invokeResult.chatRequest = request
	invokeResult.chatResponse = response

	return invokeResult, validInvokeResult(invokeResult)
}
