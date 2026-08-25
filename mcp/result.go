package mcp

import (
	"encoding/json"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type remoteResult struct {
	toolName string
	value    *sdkmcp.CallToolResult
}

func (r remoteResult) unwrap() (string, error) {
	if r.value == nil {
		return "", fmt.Errorf("mcp: call tool %q: server returned a nil result", r.toolName)
	}
	if r.value.IsError {
		return "", &ToolCallError{
			ToolName: r.toolName,
			Message:  r.firstText("tool returned isError=true with no text content"),
		}
	}
	return r.content()
}

func (r remoteResult) content() (string, error) {
	if len(r.value.Content) == 0 {
		return "", nil
	}
	if len(r.value.Content) == 1 {
		if text, ok := r.value.Content[0].(*sdkmcp.TextContent); ok {
			return text.Text, nil
		}
	}
	encoded, err := json.Marshal(r.value.Content)
	if err != nil {
		return "", fmt.Errorf("mcp: encode tool content: %w", err)
	}
	return string(encoded), nil
}

func (r remoteResult) firstText(fallback string) string {
	for _, content := range r.value.Content {
		if text, ok := content.(*sdkmcp.TextContent); ok && text.Text != "" {
			return text.Text
		}
	}
	return fallback
}
