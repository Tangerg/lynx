package turn

import (
	"reflect"
	"testing"
)

func TestDecodeToolResult(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   any
	}{
		{name: "empty"},
		{name: "object", output: `{"stdout":"ok","exit_code":0}`, want: map[string]any{
			"stdout": "ok", "exit_code": float64(0),
		}},
		{name: "plain text", output: "denied", want: "denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decodeToolResult(test.output); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("decodeToolResult = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestToolOutputText(t *testing.T) {
	result := map[string]any{"stdout": "out", "stderr": "err"}
	if got := toolOutputText("shell", result); got != "out\nerr" {
		t.Fatalf("shell output = %q, want %q", got, "out\nerr")
	}
	if got := toolOutputText("grep", result); got != "" {
		t.Fatalf("grep output = %q, want empty", got)
	}
}
