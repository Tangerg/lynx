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
}

type typedNilToolResolver struct{}

func (typedNilToolResolver) Resolve(string) (tools.Tool, bool) {
	var tool *protocolTool
	return tool, true
}

func (t *protocolTool) Definition() chat.ToolDefinition { return t.definition }

func (*protocolTool) Call(context.Context, string) (string, error) { return "ok", nil }

func protocolRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	registry, err := tools.NewRegistry(&protocolTool{definition: chat.ToolDefinition{
		Name:        "lookup",
		Description: "look up a value",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}})
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

func TestInvocationCombinesProtocolAndRuntime(t *testing.T) {
	registry := protocolRegistry(t)
	request := protocolRequest(t)
	request.Tools = registry.Definitions()

	invocation, err := toolloop.NewInvocation(request, registry)
	if err != nil {
		t.Fatalf("NewInvocation: %v", err)
	}
	if invocation.Request != request || invocation.Tools != registry {
		t.Fatalf("Invocation = %#v, want supplied request and resolver", invocation)
	}
	if err := invocation.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if _, err := json.Marshal(invocation); !errors.Is(err, toolloop.ErrInvocationNotSerializable) {
		t.Fatalf("Marshal error = %v", err)
	}
	body, err := json.Marshal(invocation.Request)
	if err != nil || len(body) == 0 {
		t.Fatalf("Request Marshal = %q, %v", body, err)
	}
}

func TestInvocationValidation(t *testing.T) {
	requestWithTool := protocolRequest(t)
	requestWithTool.Tools = protocolRegistry(t).Definitions()

	for _, test := range []struct {
		name       string
		invocation *toolloop.Invocation
	}{
		{name: "nil receiver"},
		{name: "missing request", invocation: &toolloop.Invocation{}},
		{name: "invalid request", invocation: &toolloop.Invocation{Request: &chat.Request{}}},
		{name: "missing resolver", invocation: &toolloop.Invocation{Request: requestWithTool}},
		{name: "unresolved tool", invocation: &toolloop.Invocation{Request: requestWithTool, Tools: &tools.Registry{}}},
		{name: "typed nil tool", invocation: &toolloop.Invocation{Request: requestWithTool, Tools: typedNilToolResolver{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.invocation.Validate(); !errors.Is(err, toolloop.ErrInvalidInvocation) {
				t.Fatalf("Validate error = %v", err)
			}
		})
	}

	requestWithoutTools := protocolRequest(t)
	if _, err := toolloop.NewInvocation(requestWithoutTools, nil); err != nil {
		t.Fatalf("NewInvocation without tools: %v", err)
	}
}
