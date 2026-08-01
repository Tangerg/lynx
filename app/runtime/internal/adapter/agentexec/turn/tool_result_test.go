package turn

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

type testToolPresenter struct{}

func (testToolPresenter) Activity(string) string { return "Presenting tool" }

func (testToolPresenter) Present(_ string, _ tool.Arguments, _ tool.Result) (tool.Result, string) {
	return tool.StringResult("presented"), "plain output"
}

func TestDecodeToolResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   any
	}{
		{name: "empty"},
		{name: "object", output: `{"stdout":"ok","exit_code":0}`, want: map[string]any{
			"stdout": "ok", "exit_code": json.Number("0"),
		}},
		{name: "plain text", output: "denied", want: "denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, outputText := decodeToolResult(nil, "shell", `{}`, test.output)
			if outputText != "" {
				t.Fatalf("output text = %q, want empty", outputText)
			}
			if got == nil && test.want == nil {
				return
			}
			if got == nil || !reflect.DeepEqual(got.Any(), test.want) {
				t.Fatalf("decodeToolResult = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDecodeToolResultUsesInjectedPresenter(t *testing.T) {
	result, outputText := decodeToolResult(testToolPresenter{}, "custom", `{}`, `{"value":1}`)
	if result == nil {
		t.Fatal("result is nil")
	}
	if got, _ := result.String(); got != "presented" {
		t.Fatalf("result = %v, want presented", result.Any())
	}
	if outputText != "plain output" {
		t.Fatalf("output text = %q, want plain output", outputText)
	}
}

func TestToolActivityUsesPresenterAndGenericFallback(t *testing.T) {
	presented := &memoryDispatcher{toolPresenter: testToolPresenter{}}
	if got := presented.toolActivity("custom"); got != "Presenting tool" {
		t.Fatalf("presented activity = %q", got)
	}
	generic := new(memoryDispatcher)
	if got := generic.toolActivity("custom"); got != "Calling custom" {
		t.Fatalf("generic activity = %q", got)
	}
}
