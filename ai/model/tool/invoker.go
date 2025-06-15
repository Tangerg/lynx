package tool

import (
	stdContext "context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/Tangerg/lynx/ai/model/chat/response"
)

type invoker struct {
	registry *Registry
}

func newInvoker(registry *Registry) *invoker {
	return &invoker{
		registry: registry,
	}
}

func (e *invoker) shouldInvokeToolCalls(chatResponse *response.ChatResponse) (bool, error) {
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

func (e *invoker) buildContext(ctx stdContext.Context) Context {
	return NewContext(ctx)
}

func (e *invoker) invokeToolCalls(ctx Context, toolCalls []*messages.ToolCall) (*InvokeResultBuilder, error) {
	var (
		externalToolCalls []*messages.ToolCall
		returnDirect      = true
		toolCallResponses []*messages.ToolResponse
	)

	for _, toolCall := range toolCalls {
		// always true by shouldInvokeToolCalls precheck
		t, _ := e.registry.Find(toolCall.Name)
		ct, ok := t.(CallableTool)
		if !ok {
			externalToolCalls = append(externalToolCalls, toolCall)
			continue
		}

		resp, err := ct.Call(ctx, toolCall.Arguments)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to invoke tool call %s ", toolCall.Name))
		}

		returnDirect = returnDirect && ct.Metadata().ReturnDirect()
		toolCallResponses = append(toolCallResponses, &messages.ToolResponse{
			ID:           toolCall.ID,
			Name:         toolCall.Name,
			ResponseData: resp,
		})
	}

	toolMessage, err := messages.NewToolResponseMessage(toolCallResponses, nil)
	if err != nil {
		return nil, err
	}

	return NewInvokeResultBuilder().
			WithToolResponseMessage(toolMessage).
			WithExternalToolCalls(externalToolCalls...).
			WithReturnDirect(returnDirect),
		nil
}

func (e *invoker) invoke(ctx stdContext.Context, req *request.ChatRequest, resp *response.ChatResponse) (*InvokeResult, error) {
	shouldInvokeToolCalls, err := e.shouldInvokeToolCalls(resp)
	if err != nil {
		return nil, err
	}
	if !shouldInvokeToolCalls {
		return nil, errors.New("cannot invoke tool calls")
	}

	//always not nil by shouldInvokeToolCalls precheck
	chatResult := resp.FirstToolCallsResult()

	executionResultBuilder, err := e.invokeToolCalls(
		e.buildContext(ctx),
		chatResult.Output().ToolCalls(),
	)
	if err != nil {
		return nil, err
	}

	return executionResultBuilder.
		WithChatRequest(req).
		WithChatResponse(resp).
		Build()
}
