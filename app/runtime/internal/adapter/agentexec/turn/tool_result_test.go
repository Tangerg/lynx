package turn

import (
	"encoding/json"
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
			"stdout": "ok", "exit_code": json.Number("0"),
		}},
		{name: "plain text", output: "denied", want: "denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := decodeToolResult(test.output)
			if got == nil && test.want == nil {
				return
			}
			if got == nil || !reflect.DeepEqual(got.Any(), test.want) {
				t.Fatalf("decodeToolResult = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestToolOutputText(t *testing.T) {
	encoded := decodeToolResult(`{"stdout":"out","stderr":"err"}`)
	if got := toolOutputText("shell", encoded); got != "out\nerr" {
		t.Fatalf("shell output = %q, want %q", got, "out\nerr")
	}
	if got := toolOutputText("grep", encoded); got != "" {
		t.Fatalf("grep output = %q, want empty", got)
	}
}
