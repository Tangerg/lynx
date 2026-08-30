package chatclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

var ErrInvalidToolMiddleware = errors.New("chatclient: invalid tool middleware")

type toolMiddleware struct {
	bindings    map[string]tool.Binding
	definitions []chat.ToolDefinition
}

// NewToolMiddleware keeps direct Client usage useful for the small case where
// model-requested Tools only need schema validation and serial execution. Agent
// control flow and execution policy deliberately remain outside this boundary.
func NewToolMiddleware(executables ...tool.Tool) (chat.CallMiddleware, error) {
	if len(executables) == 0 {
		return nil, fmt.Errorf("%w: at least one Tool is required", ErrInvalidToolMiddleware)
	}
	middleware := &toolMiddleware{
		bindings:    make(map[string]tool.Binding, len(executables)),
		definitions: make([]chat.ToolDefinition, 0, len(executables)),
	}
	for index, executable := range executables {
		binding, err := tool.Bind(executable)
		if err != nil {
			return nil, fmt.Errorf("%w: Tools[%d]: %w", ErrInvalidToolMiddleware, index, err)
		}
		definition := binding.Definition()
		if _, duplicate := middleware.bindings[definition.Name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate Tool name %q", ErrInvalidToolMiddleware, definition.Name)
		}
		middleware.bindings[definition.Name] = binding
		middleware.definitions = append(middleware.definitions, definition)
	}
	return middleware.wrap, nil
}

func (m *toolMiddleware) wrap(next chat.Model) chat.Model {
	return chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		return m.call(ctx, next, request)
	})
}

func (m *toolMiddleware) call(
	ctx context.Context,
	next chat.Model,
	request *chat.Request,
) (*chat.Response, error) {
	if len(request.Tools) != 0 {
		return nil, fmt.Errorf(
			"%w: Request.Tools must be empty because the middleware owns the executable Tool manifest",
			ErrInvalidToolMiddleware,
		)
	}
	current := request.Clone()
	current.Tools = m.cloneDefinitions()
	for {
		response, err := next.Call(ctx, current)
		if err != nil {
			return nil, err
		}
		calls, err := toolCalls(response)
		if err != nil {
			return nil, err
		}
		if len(calls) == 0 {
			return response, nil
		}
		results, err := m.execute(ctx, calls)
		if err != nil {
			return nil, err
		}
		current.Messages = append(
			current.Messages,
			response.Output.Message.Clone(),
			chat.NewToolMessage(results...),
		)
	}
}

func (m *toolMiddleware) execute(ctx context.Context, calls []chat.ToolCall) ([]chat.ToolResult, error) {
	results := make([]chat.ToolResult, 0, len(calls))
	for index, call := range calls {
		binding, exists := m.bindings[call.Name]
		if !exists {
			return nil, fmt.Errorf("chatclient: execute ToolCall[%d]: Tool %q is not bound", index, call.Name)
		}
		invocation, err := binding.Prepare(call)
		if err != nil {
			return nil, fmt.Errorf("chatclient: prepare ToolCall[%d]: %w", index, err)
		}
		output, err := binding.Call(ctx, invocation)
		if err != nil {
			return nil, fmt.Errorf("chatclient: execute ToolCall[%d] %q: %w", index, call.Name, err)
		}
		if err := output.Validate(); err != nil {
			return nil, fmt.Errorf("chatclient: validate ToolCall[%d] %q output: %w", index, call.Name, err)
		}
		results = append(results, chat.ToolResult{ID: call.ID, Name: call.Name, Output: output.Clone()})
	}
	return results, nil
}

func (m *toolMiddleware) cloneDefinitions() []chat.ToolDefinition {
	definitions := make([]chat.ToolDefinition, len(m.definitions))
	for index := range m.definitions {
		definitions[index] = m.definitions[index].Clone()
	}
	return definitions
}

func toolCalls(response *chat.Response) ([]chat.ToolCall, error) {
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("chatclient: Tool middleware received an invalid response: %w", err)
	}
	if response.Output == nil || response.Output.Message == nil {
		return nil, nil
	}
	var calls []chat.ToolCall
	for index, part := range response.Output.Message.Parts {
		switch part.Kind {
		case chat.PartToolCall:
			calls = append(calls, *part.ToolCall)
		case chat.PartToolCallDelta:
			return nil, fmt.Errorf(
				"chatclient: Tool middleware requires complete ToolCalls; response part[%d] is a delta",
				index,
			)
		}
	}
	return calls, nil
}
