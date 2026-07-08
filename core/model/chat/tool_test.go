package chat_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewTool_RequiresNameSchemaAndExec(t *testing.T) {
	nop := func(context.Context, string) (string, error) { return "", nil }

	_, err := chat.NewTool(chat.ToolDefinition{}, nop)
	if err == nil {
		t.Fatal("missing name must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search"}, nop)
	if err == nil {
		t.Fatal("missing schema must error")
	}

	_, err = chat.NewTool(chat.ToolDefinition{Name: "search", InputSchema: "{}"}, nil)
	if err == nil {
		t.Fatal("nil execFunc must error")
	}
}

func TestNewTool_RunsExecFunc(t *testing.T) {
	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "echo", InputSchema: "{}"},
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

func TestNewJSONTool_DerivesSchemaAndDecodesArguments(t *testing.T) {
	type addInput struct {
		A int `json:"a" jsonschema:"required"`
		B int `json:"b" jsonschema:"required"`
	}

	tool, err := chat.NewJSONTool[addInput](
		chat.ToolDefinition{Name: "add", Description: "add two numbers"},
		func(_ context.Context, in addInput) (string, error) {
			return strconv.Itoa(in.A + in.B), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	def := tool.Definition()
	if def.InputSchema == "" || !strings.Contains(def.InputSchema, `"a"`) || !strings.Contains(def.InputSchema, `"b"`) {
		t.Fatalf("InputSchema = %q, want schema for addInput", def.InputSchema)
	}

	got, err := tool.Call(context.Background(), `{"a":2,"b":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "5" {
		t.Fatalf("Call = %q, want 5", got)
	}
}

func TestNewJSONTool_RejectsManualSchemaAndNilExec(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}

	_, err := chat.NewJSONTool[input](
		chat.ToolDefinition{},
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err == nil {
		t.Fatal("missing name must error")
	}

	_, err = chat.NewJSONTool[input](
		chat.ToolDefinition{Name: "echo", InputSchema: "{}"},
		func(context.Context, input) (string, error) { return "", nil },
	)
	if err == nil {
		t.Fatal("manual schema must error")
	}

	_, err = chat.NewJSONTool[input](chat.ToolDefinition{Name: "echo"}, nil)
	if err == nil {
		t.Fatal("nil execFunc must error")
	}
}

func TestNewJSONTool_InvalidArguments(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}

	tool, err := chat.NewJSONTool[input](
		chat.ToolDefinition{Name: "echo"},
		func(_ context.Context, in input) (string, error) { return in.Value, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tool.Call(context.Background(), `{`); err == nil {
		t.Fatal("invalid JSON must error")
	}
}
