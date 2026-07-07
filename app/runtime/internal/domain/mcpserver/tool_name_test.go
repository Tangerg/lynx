package mcpserver

import "testing"

func TestToolName(t *testing.T) {
	tests := []struct {
		name   string
		server string
		tool   string
		want   string
	}{
		{name: "prefixes server", server: "github", tool: "read", want: "github_read"},
		{name: "sanitizes unsupported bytes", server: "html.to.design", tool: "import-url", want: "html_to_design_import-url"},
		{name: "bare tool when server empty", tool: "ping", want: "ping"},
		{name: "caps at provider limit", server: "srv", tool: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijkl", want: "srv_abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefgh"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToolName(tc.server, tc.tool); got != tc.want {
				t.Fatalf("ToolName(%q, %q) = %q, want %q", tc.server, tc.tool, got, tc.want)
			}
		})
	}
}
