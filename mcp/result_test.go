package mcp

import (
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
		wantText   string
		wantParts  int
		wantMedia  bool
	}{
		{name: "empty"},
		{name: "single text", content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "hi"}}, wantText: "hi", wantParts: 1},
		{name: "structured fallback", structured: map[string]any{"answer": 42}, wantText: `{"answer":42}`},
		{
			name:       "content precedes structured fallback",
			content:    []sdkmcp.Content{&sdkmcp.TextContent{Text: "visible"}},
			structured: map[string]any{"answer": 42},
			wantText:   "visible",
			wantParts:  1,
		},
		{
			name: "multiple",
			content: []sdkmcp.Content{
				&sdkmcp.TextContent{Text: "a"},
				&sdkmcp.TextContent{Text: "b"},
			},
			wantText:  "ab",
			wantParts: 2,
		},
		{
			name:      "single non-text",
			content:   []sdkmcp.Content{&sdkmcp.ImageContent{MIMEType: "image/png", Data: []byte{1, 2, 3}}},
			wantParts: 1,
			wantMedia: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (remoteResult{value: &sdkmcp.CallToolResult{
				Content:           test.content,
				StructuredContent: test.structured,
			}}).content()
			require.NoError(t, err)
			assert.Len(t, got.Content, test.wantParts)
			text, textOK := got.Text()
			if test.wantMedia {
				assert.False(t, textOK)
				assert.Equal(t, "media", string(got.Content[0].Kind))
				return
			}
			assert.True(t, textOK)
			assert.Equal(t, test.wantText, text)
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
