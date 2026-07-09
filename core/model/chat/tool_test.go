package chat_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestNewTool_RequiresNameEmptySchemaAndExec(t *testing.T) {
	type in struct {
		Q string `json:"q"`
	}
	run := func(context.Context, in) (string, error) { return "", nil }

	if _, err := chat.NewTool[in, string](chat.ToolDefinition{}, run); err == nil {
		t.Fatal("missing name must error")
	}
	if _, err := chat.NewTool[in, string](chat.ToolDefinition{Name: "search", InputSchema: "{}"}, run); err == nil {
		t.Fatal("non-empty InputSchema must error (it is derived from In)")
	}
	if _, err := chat.NewTool[in, string](chat.ToolDefinition{Name: "search"}, nil); err == nil {
		t.Fatal("nil execFunc must error")
	}
}

func TestNewTool_DerivesSchemaAndDecodesArguments(t *testing.T) {
	type addInput struct {
		A int `json:"a" jsonschema:"required"`
		B int `json:"b" jsonschema:"required"`
	}

	// Out = int: a non-string result is JSON-encoded to the string the model sees.
	tool, err := chat.NewTool[addInput, int](
		chat.ToolDefinition{Name: "add", Description: "add two numbers"},
		func(_ context.Context, in addInput) (int, error) { return in.A + in.B, nil },
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

func TestNewTool_StringResultPassesThroughVerbatim(t *testing.T) {
	type in struct {
		V string `json:"v"`
	}
	tool, err := chat.NewTool[in, string](
		chat.ToolDefinition{Name: "echo"},
		func(_ context.Context, i in) (string, error) { return i.V, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Call(context.Background(), `{"v":"hi"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi" {
		t.Fatalf("Call = %q, want hi (string result verbatim, not JSON-quoted)", got)
	}
}

func TestNewTool_EmptyResultReturnsDefaultSuccess(t *testing.T) {
	tool, err := chat.NewTool[struct{}, string](
		chat.ToolDefinition{Name: "noop"},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Call(context.Background(), ``)
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("an empty result must be replaced with a default success message")
	}
}

func TestNewTool_EmptyArgumentsDecodeToZeroValue(t *testing.T) {
	type input struct {
		Value string `json:"value,omitempty"`
	}

	tool, err := chat.NewTool[input, string](
		chat.ToolDefinition{Name: "opt"},
		func(_ context.Context, in input) (string, error) { return "value=" + in.Value, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	// Providers may emit "" (or whitespace) for a parameterless / all-optional
	// call; it must decode to the zero value, not error.
	for _, args := range []string{"", "   ", "{}"} {
		got, err := tool.Call(context.Background(), args)
		if err != nil {
			t.Fatalf("Call(%q) errored: %v", args, err)
		}
		if got != "value=" {
			t.Fatalf("Call(%q) = %q, want %q", args, got, "value=")
		}
	}
}

func TestNewTool_InvalidArguments(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}

	tool, err := chat.NewTool[input, string](
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
