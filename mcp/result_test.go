package mcp

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteResultContent(t *testing.T) {
	tests := []struct {
		name       string
		content    []sdkmcp.Content
		structured any
		want       string
	}{
		{name: "empty"},
		{name: "single text", content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "hi"}}, want: "hi"},
		{name: "structured fallback", structured: map[string]any{"answer": 42}, want: `{"answer":42}`},
		{
			name:       "content precedes structured fallback",
			content:    []sdkmcp.Content{&sdkmcp.TextContent{Text: "visible"}},
			structured: map[string]any{"answer": 42},
			want:       "visible",
		},
		{
			name: "multiple",
			content: []sdkmcp.Content{
				&sdkmcp.TextContent{Text: "a"},
				&sdkmcp.TextContent{Text: "b"},
			},
		},
		{
			name:    "single non-text",
			content: []sdkmcp.Content{&sdkmcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (remoteResult{value: &sdkmcp.CallToolResult{
				Content:           test.content,
				StructuredContent: test.structured,
			}}).content()
			require.NoError(t, err)
			if test.want != "" || len(test.content) == 0 {
				assert.Equal(t, test.want, got)
				return
			}
			var decoded []map[string]any
			require.NoError(t, json.Unmarshal([]byte(got), &decoded))
			require.Len(t, decoded, len(test.content))
			assert.NotEmpty(t, decoded[0]["type"])
		})
	}
}

func TestRemoteResultErrorMessage(t *testing.T) {
	result := remoteResult{value: &sdkmcp.CallToolResult{Content: []sdkmcp.Content{
		&sdkmcp.TextContent{},
		&sdkmcp.TextContent{Text: "real"},
	}}}
	assert.Equal(t, "real", result.firstText("fallback"))
	assert.Equal(t, "fallback", (remoteResult{value: &sdkmcp.CallToolResult{}}).firstText("fallback"))
}
