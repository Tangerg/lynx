package agentexec

import (
	"context"
	"iter"
	"strings"

	"github.com/Tangerg/scope/core/chat"
)

// delegatingStubModel drives one root-to-child execution through the Agent
// Framework Delegate effect and the production execution boundary.
type delegatingStubModel struct{ defaults *chat.Options }

func newDelegatingStubModel() *delegatingStubModel {
	return &delegatingStubModel{defaults: &chat.Options{Model: "stub-delegating"}}
}

func (d *delegatingStubModel) DefaultOptions() chat.Options { return *d.defaults }

func (*delegatingStubModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolMessage(request.Messages):
		return interactionUsageTextResponse("main: subtask done", 2, 1), nil
	case userMessagesContain(request.Messages, "delegate"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_once", Name: "delegate_task",
			Arguments: `{"summary":"delegated work","instructions":"do the subtask"}`,
		}, 2, 1), nil
	default:
		return interactionUsageTextResponse("subtask: result", 2, 1), nil
	}
}

func (d *delegatingStubModel) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	response, err := d.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

// nestedDelegatingStub drives root → child → grandchild, proving that each
// Agent Framework Process owns its own interaction context and product Run lineage.
type nestedDelegatingStub struct{ defaults *chat.Options }

func newNestedDelegatingStub() *nestedDelegatingStub {
	return &nestedDelegatingStub{defaults: &chat.Options{Model: "stub-nested-delegating"}}
}

func (n *nestedDelegatingStub) DefaultOptions() chat.Options { return *n.defaults }

func (*nestedDelegatingStub) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	switch {
	case hasToolMessage(request.Messages) && userMessagesContain(request.Messages, "child delegate"):
		return interactionUsageTextResponse("child: result", 2, 1), nil
	case hasToolMessage(request.Messages):
		return interactionUsageTextResponse("root: result", 2, 1), nil
	case userMessagesContain(request.Messages, "nested root"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_child", Name: "delegate_task",
			Arguments: `{"summary":"delegated work","instructions":"child delegate"}`,
		}, 2, 1), nil
	case userMessagesContain(request.Messages, "child delegate"):
		return interactionToolResponse(chat.ToolCall{
			ID: "delegate_grandchild", Name: "delegate_task",
			Arguments: `{"summary":"delegated work","instructions":"grandchild leaf"}`,
		}, 2, 1), nil
	default:
		return interactionUsageTextResponse("grandchild: result", 2, 1), nil
	}
}

func (n *nestedDelegatingStub) Stream(
	ctx context.Context,
	request *chat.Request,
) iter.Seq2[*chat.Response, error] {
	response, err := n.Call(ctx, request)
	return func(yield func(*chat.Response, error) bool) { yield(response, err) }
}

func hasToolMessage(messages []chat.Message) bool {
	for _, message := range messages {
		if message.Role == chat.RoleTool {
			return true
		}
	}
	return false
}

func userMessagesContain(messages []chat.Message, substring string) bool {
	for _, message := range messages {
		if message.Role == chat.RoleUser && strings.Contains(message.Text(), substring) {
			return true
		}
	}
	return false
}

func toolResult(messages []chat.Message, name string) string {
	for _, message := range messages {
		if message.Role != chat.RoleTool {
			continue
		}
		for _, part := range message.Parts {
			if part.Kind == chat.PartToolResult && part.ToolResult != nil && part.ToolResult.Name == name {
				return part.ToolResult.Result
			}
		}
	}
	return ""
}
