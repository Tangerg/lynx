package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/tools"
)

type addInput struct {
	A int `json:"a" jsonschema:"required"`
	B int `json:"b" jsonschema:"required"`
}

type contextKey struct{}

func TestNewBuildsImmutableTypedFunctionTool(t *testing.T) {
	tool, err := tools.New(tools.Config{Name: "add", Description: "add two integers"},
		func(ctx context.Context, input addInput) (int, error) {
			if got := ctx.Value(contextKey{}); got != "value" {
				t.Fatalf("context value = %v", got)
			}
			return input.A + input.B, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	definition := tool.Definition()
	if definition.Name != "add" || definition.Description != "add two integers" {
		t.Fatalf("definition = %+v", definition)
	}
	if schema := string(definition.InputSchema); !strings.Contains(schema, `"a"`) || !strings.Contains(schema, `"b"`) {
		t.Fatalf("schema = %s", schema)
	}
	definition.InputSchema[0] = '['
	if tool.Definition().InputSchema[0] != '{' {
		t.Fatal("mutating returned definition changed the tool")
	}

	ctx := context.WithValue(t.Context(), contextKey{}, "value")
	result, err := tool.Call(ctx, `{"a":2,"b":3}`)
	if err != nil || result != "5" {
		t.Fatalf("Call = %q, %v", result, err)
	}

	registry, err := tools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	if resolved, ok := registry.Resolve("add"); !ok || resolved != tool {
		t.Fatalf("Resolve(add) = %v, %v", resolved, ok)
	}
}

func TestNewRejectsInvalidConstruction(t *testing.T) {
	valid := func(context.Context, struct{}) (string, error) { return "", nil }
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "missing name", run: func() error {
			_, err := tools.New(tools.Config{}, valid)
			return err
		}},
		{name: "whitespace name", run: func() error {
			_, err := tools.New(tools.Config{Name: "bad name"}, valid)
			return err
		}},
		{name: "nil function", run: func() error {
			_, err := tools.New[struct{}, string](tools.Config{Name: "nil"}, nil)
			return err
		}},
		{name: "scalar input", run: func() error {
			_, err := tools.New(tools.Config{Name: "scalar"}, func(context.Context, string) (string, error) { return "", nil })
			return err
		}},
		{name: "interface input", run: func() error {
			_, err := tools.New(tools.Config{Name: "interface"}, func(context.Context, any) (string, error) { return "", nil })
			return err
		}},
		{name: "pointer chain input", run: func() error {
			_, err := tools.New(tools.Config{Name: "pointers"}, func(context.Context, **addInput) (string, error) { return "", nil })
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, tools.ErrInvalidTool) {
				t.Fatalf("error = %v, want ErrInvalidTool", err)
			}
		})
	}
}

func TestFunctionToolDecodesStrictObjectArguments(t *testing.T) {
	type optionalInput struct {
		Value string `json:"value,omitempty"`
	}
	tool, err := tools.New(tools.Config{Name: "optional"},
		func(_ context.Context, input *optionalInput) (string, error) {
			if input == nil {
				t.Fatal("pointer input was not allocated")
			}
			return input.Value, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	for _, arguments := range []string{"", "  ", "{}"} {
		result, err := tool.Call(t.Context(), arguments)
		if err != nil || result != "" {
			t.Fatalf("Call(%q) = %q, %v", arguments, result, err)
		}
	}
	for _, arguments := range []string{
		`null`,
		`[]`,
		`"text"`,
		`{"unknown":true}`,
		`{} {}`,
		`{`,
	} {
		if _, err := tool.Call(t.Context(), arguments); err == nil {
			t.Errorf("Call(%q) succeeded, want decode error", arguments)
		}
	}
}

func TestFunctionToolResultEncodingAndErrorIdentity(t *testing.T) {
	t.Run("defined string is verbatim", func(t *testing.T) {
		type text string
		tool, err := tools.New(tools.Config{Name: "text"}, func(context.Context, struct{}) (text, error) {
			return "plain", nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := tool.Call(t.Context(), `{}`); err != nil || got != "plain" {
			t.Fatalf("Call = %q, %v", got, err)
		}
	})

	t.Run("composite and raw JSON use JSON encoding", func(t *testing.T) {
		type output struct {
			OK bool `json:"ok"`
		}
		composite, err := tools.New(tools.Config{Name: "composite"}, func(context.Context, struct{}) (output, error) {
			return output{OK: true}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := composite.Call(t.Context(), `{}`); err != nil || got != `{"ok":true}` {
			t.Fatalf("composite Call = %q, %v", got, err)
		}

		raw, err := tools.New(tools.Config{Name: "raw"}, func(context.Context, struct{}) (json.RawMessage, error) {
			return json.RawMessage(`{"ok":true}`), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := raw.Call(t.Context(), `{}`); err != nil || got != `{"ok":true}` {
			t.Fatalf("raw Call = %q, %v", got, err)
		}
	})

	t.Run("empty output stays empty", func(t *testing.T) {
		tool, err := tools.New(tools.Config{Name: "empty"}, func(context.Context, struct{}) (string, error) {
			return "", nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := tool.Call(t.Context(), `{}`); err != nil || got != "" {
			t.Fatalf("Call = %q, %v", got, err)
		}
	})

	t.Run("function error is preserved", func(t *testing.T) {
		want := errors.New("failed")
		tool, err := tools.New(tools.Config{Name: "error"}, func(context.Context, struct{}) (string, error) {
			return "partial", want
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := tool.Call(t.Context(), `{}`); got != "" || !errors.Is(err, want) {
			t.Fatalf("Call = %q, %v", got, err)
		}
	})

	t.Run("encoding error is wrapped", func(t *testing.T) {
		tool, err := tools.New(tools.Config{Name: "channel"}, func(context.Context, struct{}) (chan int, error) {
			return make(chan int), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tool.Call(t.Context(), `{}`); err == nil || !strings.Contains(err.Error(), "encode result") {
			t.Fatalf("Call error = %v", err)
		}
	})
}

func TestFunctionToolConcurrentCalls(t *testing.T) {
	tool, err := tools.New(tools.Config{Name: "concurrent"}, func(_ context.Context, input addInput) (int, error) {
		return input.A + input.B, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	for range 32 {
		wait.Go(func() {
			if got, err := tool.Call(t.Context(), `{"a":1,"b":2}`); err != nil || got != "3" {
				t.Errorf("Call = %q, %v", got, err)
			}
		})
	}
	wait.Wait()
}
