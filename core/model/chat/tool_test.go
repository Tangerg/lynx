package chat_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewTool_RequiresNameSchemaAndExec(t *testing.T) {
	nop := func(context.Context, string) (string, error) { return "", nil }

	_, err := chat.NewTool(chat.ToolDefinition{}, chat.ToolMetadata{}, nop)
	if err == nil {
		t.Fatal("missing name must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search"}, chat.ToolMetadata{}, nop)
	if err == nil {
		t.Fatal("missing schema must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search", InputSchema: "{}"}, chat.ToolMetadata{}, nil)
	if err == nil {
		t.Fatal("nil execFunc must error")
	}
}

func TestNewTool_RunsExecFunc(t *testing.T) {
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "echo", InputSchema: "{}"},
		chat.ToolMetadata{},
		func(_ context.Context, args string) (string, error) { return args, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Call(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi" {
		t.Fatalf("Call = %q, want hi", got)
	}
}
