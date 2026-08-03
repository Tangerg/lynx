package tool_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func TestNewFuncUsesExplicitDefinition(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	definition := chat.ToolDefinition{
		Name:        "echo",
		Description: "echo a value",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}}}`),
	}
	function, err := tool.NewFunc(definition, func(_ context.Context, input input) (string, error) {
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	definition.InputSchema[0] = '['
	if got := function.Definition(); got.Name != "echo" || got.InputSchema[0] != '{' {
		t.Fatalf("Definition() = %+v", got)
	}
	if got, err := function.Call(t.Context(), `{"value":"ok"}`); err != nil || got != "ok" {
		t.Fatalf("Call() = %q, %v", got, err)
	}
	if _, err := function.Call(t.Context(), `{"extra":true}`); err == nil {
		t.Fatal("Call accepted an unknown field")
	}
}

func TestNewFuncRejectsInvalidConstruction(t *testing.T) {
	definition := chat.ToolDefinition{Name: "valid", InputSchema: json.RawMessage(`{}`)}
	valid := func(context.Context, struct{}) (string, error) { return "", nil }
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "invalid definition", run: func() error {
			_, err := tool.NewFunc(chat.ToolDefinition{}, valid)
			return err
		}},
		{name: "nil function", run: func() error {
			_, err := tool.NewFunc[struct{}, string](definition, nil)
			return err
		}},
		{name: "scalar input", run: func() error {
			_, err := tool.NewFunc(definition, func(context.Context, string) (string, error) { return "", nil })
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, tool.ErrInvalidTool) {
				t.Fatalf("error = %v, want ErrInvalidTool", err)
			}
		})
	}
}

func TestFuncEncodesResultsAndPreservesErrors(t *testing.T) {
	definition := chat.ToolDefinition{Name: "result", InputSchema: json.RawMessage(`{}`)}
	want := errors.New("failed")
	failing, err := tool.NewFunc(definition, func(context.Context, struct{}) (string, error) { return "partial", want })
	if err != nil {
		t.Fatal(err)
	}
	if got, err := failing.Call(t.Context(), `{}`); got != "" || !errors.Is(err, want) {
		t.Fatalf("Call() = %q, %v", got, err)
	}

	unencodable, err := tool.NewFunc(definition, func(context.Context, struct{}) (chan int, error) {
		return make(chan int), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unencodable.Call(t.Context(), `{}`); err == nil || !strings.Contains(err.Error(), "encode function result") {
		t.Fatalf("Call error = %v", err)
	}
}
