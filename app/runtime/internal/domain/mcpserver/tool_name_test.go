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

func TestToolRefPreservesIdentityAcrossPublicNameCollision(t *testing.T) {
	first := ToolRef{Server: "a_b", Tool: "c"}
	second := ToolRef{Server: "a", Tool: "b_c"}
	if first == second {
		t.Fatal("distinct tool references compared equal")
	}
	if first.PublicName() != second.PublicName() {
		t.Fatalf("fixture public names do not collide: %q != %q", first.PublicName(), second.PublicName())
	}
}
