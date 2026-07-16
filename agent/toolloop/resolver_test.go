package toolloop_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/toolloop"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

var _ toolloop.ToolResolver = (*tools.Registry)(nil)

type protocolTool struct {
	definition chat.ToolDefinition
	call       func(context.Context, string) (string, error)
}

type typedNilToolResolver struct{}

func (typedNilToolResolver) Resolve(string) (tools.Tool, bool) {
	var tool *protocolTool
	return tool, true
}

func (t *protocolTool) Definition() chat.ToolDefinition { return t.definition }

func (t *protocolTool) Call(ctx context.Context, arguments string) (string, error) {
	if t.call != nil {
		return t.call(ctx, arguments)
	}
	return "ok", nil
}

func protocolRegistry(t *testing.T) *tools.Registry {
	return protocolRegistryWithCall(t, nil)
}

func protocolRegistryWithCall(t *testing.T, call func(context.Context, string) (string, error)) *tools.Registry {
	t.Helper()
	registry, err := tools.NewRegistry(&protocolTool{definition: chat.ToolDefinition{
		Name:        "lookup",
		Description: "look up a value",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, call: call})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func protocolRequest(t *testing.T) *chat.Request {
	t.Helper()
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	return request
}

func TestRunnerValidatesAdvertisedTools(t *testing.T) {
	requestWithTool := protocolRequest(t)
	requestWithTool.Tools = protocolRegistry(t).Definitions()
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return runnerTextResponse("ok"), nil
	})
	runner := newRunner(t, model, toolloop.Config{})

	for _, test := range []struct {
		name     string
		request  *chat.Request
		resolver toolloop.ToolResolver
	}{
		{name: "missing request"},
		{name: "invalid request", request: &chat.Request{}},
		{name: "missing resolver", request: requestWithTool},
		{name: "unresolved tool", request: requestWithTool, resolver: &tools.Registry{}},
		{name: "typed nil tool", request: requestWithTool, resolver: typedNilToolResolver{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := collectRunnerEvents(runner.Run(context.Background(), test.request, test.resolver))
			if !errors.Is(err, toolloop.ErrInvalidInput) {
				t.Fatalf("Run error = %v", err)
			}
		})
	}

	requestWithoutTools := protocolRequest(t)
	if _, err := collectRunnerEvents(runner.Run(context.Background(), requestWithoutTools, nil)); err != nil {
		t.Fatalf("Run without tools: %v", err)
	}
}
