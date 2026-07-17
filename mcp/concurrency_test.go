package mcp_test

import (
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
)

func TestAnnotatedReadOnlyConcurrency(t *testing.T) {
	tests := []struct {
		name       string
		tool       *sdkmcp.Tool
		concurrent bool
	}{
		{name: "nil descriptor"},
		{name: "missing annotations", tool: &sdkmcp.Tool{}},
		{
			name: "idempotent mutation is still exclusive",
			tool: &sdkmcp.Tool{Annotations: &sdkmcp.ToolAnnotations{
				IdempotentHint: true,
			}},
		},
		{
			name: "explicit read only",
			tool: &sdkmcp.Tool{Annotations: &sdkmcp.ToolAnnotations{
				ReadOnlyHint: true,
			}},
			concurrent: true,
		},
		{
			name: "contradictory destructive hint fails closed",
			tool: &sdkmcp.Tool{Annotations: &sdkmcp.ToolAnnotations{
				ReadOnlyHint:    true,
				DestructiveHint: new(true),
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, concurrent := lynxmcp.AnnotatedReadOnlyConcurrency("source", test.tool, `{"id":"one"}`)
			if key != "" || concurrent != test.concurrent {
				t.Fatalf("AnnotatedReadOnlyConcurrency() = %q, %t, want empty key, %t", key, concurrent, test.concurrent)
			}
		})
	}
}
