package tool

import (
	"errors"
	"slices"

	"github.com/Tangerg/lynx/ai/model/chat/messages"
	"github.com/Tangerg/lynx/ai/model/chat/request"
	"github.com/Tangerg/lynx/ai/model/chat/response"
	"github.com/Tangerg/lynx/ai/model/chat/result"
)

type InvokeResult struct {
	chatRequest         *request.ChatRequest
	chatResponse        *response.ChatResponse
	toolResponseMessage *messages.ToolResponseMessage
	returnDirect        bool
	externalToolCalls   []*messages.ToolCall
}

func (e *InvokeResult) ShouldBuildChatRequest() bool {
	if len(e.externalToolCalls) > 0 {
		return false
	}
	return e.returnDirect
}

func (e *InvokeResult) ShouldBuildChatResponse() bool {
	return !e.ShouldBuildChatRequest()
}

func (e *InvokeResult) BuildChatRequest() (*request.ChatRequest, error) {
	if !e.ShouldBuildChatRequest() {
		return nil, errors.New("cannot build chat chat request")
	}
	if e.chatRequest == nil {
		return nil, errors.New("cannot build chat chat request")
	}
	if e.chatRequest == nil {
		return nil, errors.New("cannot build chat chat request")
	}
	if e.toolResponseMessage == nil {
		return nil, errors.New("cannot build chat chat request")
	}
	chatResult := e.chatResponse.FirstToolCallsResult()
	if chatResult == nil {
		return nil, errors.New("cannot build chat chat request")
	}

	opts := e.chatRequest.Options().Clone()
	history := e.chatRequest.Instructions()
	msgs := slices.Clone(history)
	msgs = append(msgs, chatResult.Output())
	msgs = append(msgs, e.toolResponseMessage)

	return request.NewChatRequest(msgs, opts)
}

func (e *InvokeResult) BuildChatResponse() (*response.ChatResponse, error) {
	if e.ShouldBuildChatResponse() {
		return nil, errors.New("cannot build chat chat response")
	}
	if e.chatResponse == nil {
		return nil, errors.New("cannot build chat chat response")
	}
	chatResult := e.chatResponse.FirstToolCallsResult()
	if chatResult == nil {
		return nil, errors.New("cannot build chat chat response")
	}

	msg := chatResult.Output()
	newMsg := messages.NewAssistantMessage(
		msg.Text(),
		msg.Media(),
		e.externalToolCalls,
		msg.Metadata(),
	)

	newResult, err := result.
		NewChatResultBuilder().
		WithAssistantMessage(newMsg).
		WithMetadata(chatResult.Metadata()).
		WithToolResponseMessage(e.toolResponseMessage).
		Build()
	if err != nil {
		return nil, err
	}

	return response.
		NewChatResponseBuilder().
		WithResult(newResult).
		WithMetadata(e.chatResponse.Metadata()).
		Build()
}

type InvokeResultBuilder struct {
	chatRequest         *request.ChatRequest
	chatResponse        *response.ChatResponse
	toolResponseMessage *messages.ToolResponseMessage
	returnDirect        bool
	externalToolCalls   []*messages.ToolCall
}

func NewInvokeResultBuilder() *InvokeResultBuilder {
	return &InvokeResultBuilder{}
}

func (e *InvokeResultBuilder) WithChatRequest(chatRequest *request.ChatRequest) *InvokeResultBuilder {
	if chatRequest != nil {
		e.chatRequest = chatRequest
	}
	return e
}

func (e *InvokeResultBuilder) WithChatResponse(chatResponse *response.ChatResponse) *InvokeResultBuilder {
	if chatResponse != nil {
		e.chatResponse = chatResponse
	}
	return e
}

func (e *InvokeResultBuilder) WithToolResponseMessage(toolResponseMessage *messages.ToolResponseMessage) *InvokeResultBuilder {
	if toolResponseMessage != nil {
		e.toolResponseMessage = toolResponseMessage
	}
	return e
}

func (e *InvokeResultBuilder) WithReturnDirect(returnDirect bool) *InvokeResultBuilder {
	e.returnDirect = returnDirect
	return e
}

func (e *InvokeResultBuilder) WithExternalToolCalls(externalToolCalls ...*messages.ToolCall) *InvokeResultBuilder {
	e.externalToolCalls = append(e.externalToolCalls, externalToolCalls...)
	return e
}

func (e *InvokeResultBuilder) Build() (*InvokeResult, error) {
	if e.chatRequest == nil {
		return nil, errors.New("chat request is required")
	}
	if e.chatResponse == nil {
		return nil, errors.New("chat response is required")
	}
	if e.toolResponseMessage == nil && len(e.externalToolCalls) == 0 {
		return nil, errors.New("tool response or external tool calls is at least one")
	}
	return &InvokeResult{
		chatRequest:         e.chatRequest,
		chatResponse:        e.chatResponse,
		toolResponseMessage: e.toolResponseMessage,
		returnDirect:        e.returnDirect,
		externalToolCalls:   e.externalToolCalls,
	}, nil
}

func (e *InvokeResultBuilder) MustBuild() *InvokeResult {
	invokeResult, err := e.Build()
	if err != nil {
		panic(err)
	}
	return invokeResult
}
