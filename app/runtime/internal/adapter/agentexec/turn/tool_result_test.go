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
			"output": "ok", "exitCode": json.Number("0"),
		}},
		{name: "plain text", output: "denied", want: "denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := decodeToolResult("shell", `{}`, test.output)
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
	encoded := decodeToolResult("shell", `{}`, `{"stdout":"out","stderr":"err"}`)
	if got := toolOutputText("shell", encoded); got != "out\nerr" {
		t.Fatalf("shell output = %q, want %q", got, "out\nerr")
	}
	if got := toolOutputText("grep", encoded); got != "" {
		t.Fatalf("grep output = %q, want empty", got)
	}
}

func TestNormalizeToolResultUsesConcreteToolSchemaAtExecutorBoundary(t *testing.T) {
	got := normalizeToolResult("edit", map[string]any{
		"file_path": "a.go", "old_string": "old\n", "new_string": "new\n",
	}, map[string]any{"replacements": 1})
	want := map[string]any{"changes": []map[string]any{{
		"path": "a.go", "status": "modified", "diff": []map[string]any{
			{"type": "deleted", "leftLine": 1, "code": "old"},
			{"type": "added", "rightLine": 1, "code": "new"},
		},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeToolResult = %#v, want %#v", got, want)
	}
}

func TestToolActivityIsProducedBeforeApplicationProjection(t *testing.T) {
	if got := toolActivity("web_search"); got != "Searching the web" {
		t.Fatalf("toolActivity = %q, want web-search activity", got)
	}
}
