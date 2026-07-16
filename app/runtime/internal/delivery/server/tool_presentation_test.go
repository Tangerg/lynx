package server

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

func TestToolResultPresentation(t *testing.T) {
	tests := []struct {
		name string
		tool transcript.ToolInvocation
		want map[string]any
	}{
		{
			name: "command",
			tool: transcript.ToolInvocation{
				Name: "shell",
				Result: map[string]any{
					"stdout": "hello\n", "stderr": "warning", "exit_code": float64(2),
				},
			},
			want: map[string]any{"exitCode": float64(2), "output": "hello\n\nwarning"},
		},
		{
			name: "local search",
			tool: transcript.ToolInvocation{
				Name: "grep",
				Result: map[string]any{"matches": []any{
					map[string]any{"path": "a.go", "line": float64(7), "text": "match"},
				}},
			},
			want: map[string]any{"hits": []any{
				map[string]any{"path": "a.go", "lineNumber": float64(7), "snippet": "match"},
			}},
		},
		{
			name: "web search",
			tool: transcript.ToolInvocation{
				Name: "web_search",
				Result: map[string]any{"results": []any{
					map[string]any{
						"title": "Lynx", "url": "https://example.com",
						"snippet": "result", "favicon_url": "https://example.com/favicon.ico",
					},
				}},
			},
			want: map[string]any{"results": []any{
				map[string]any{
					"title": "Lynx", "url": "https://example.com",
					"snippet": "result", "faviconUrl": "https://example.com/favicon.ico",
				},
			}},
		},
		{
			name: "edit",
			tool: transcript.ToolInvocation{
				Name: "edit",
				Arguments: map[string]any{
					"file_path": "a.go", "old_string": "old\n", "new_string": "new\n",
				},
				Result: map[string]any{"replacements": float64(1)},
			},
			want: map[string]any{"changes": []any{
				map[string]any{
					"path": "a.go", "status": "modified", "diff": []any{
						map[string]any{"type": "deleted", "leftLine": float64(1), "code": "old"},
						map[string]any{"type": "added", "rightLine": float64(1), "code": "new"},
					},
				},
			}},
		},
		{
			name: "write",
			tool: transcript.ToolInvocation{
				Name:      "write",
				Arguments: map[string]any{"file_path": "b.go"},
				Result:    map[string]any{"bytes_written": float64(4)},
			},
			want: map[string]any{"changes": []any{
				map[string]any{"path": "b.go", "status": "modified"},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := jsonObject(t, presentToolResult(test.tool)); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("presentToolResult = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestToolResultPresenterPreservesPresentedAndUnknownResults(t *testing.T) {
	presented := map[string]any{"output": "ready", "exitCode": float64(0)}
	if got := presentToolResult(transcript.ToolInvocation{Name: "shell", Result: presented}); !reflect.DeepEqual(got, presented) {
		t.Fatalf("presented result = %#v, want %#v", got, presented)
	}

	raw := map[string]any{"custom": true}
	if got := presentToolResult(transcript.ToolInvocation{Name: "vendor_tool", Result: raw}); !reflect.DeepEqual(got, raw) {
		t.Fatalf("unknown result = %#v, want %#v", got, raw)
	}
	if got := presentToolResult(transcript.ToolInvocation{Name: "shell", Result: raw}); !reflect.DeepEqual(got, raw) {
		t.Fatalf("unrecognized shell result = %#v, want raw result %#v", got, raw)
	}
}

func TestToolActivity(t *testing.T) {
	tests := map[string]string{
		"":            "",
		"shell":       "Running command",
		"write":       "Writing file",
		"web_search":  "Searching the web",
		"vendor_tool": "Calling vendor_tool",
	}
	for tool, want := range tests {
		if got := toolActivity(tool); got != want {
			t.Errorf("toolActivity(%q) = %q, want %q", tool, got, want)
		}
	}
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return object
}
