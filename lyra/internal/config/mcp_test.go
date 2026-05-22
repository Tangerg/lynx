package config

import (
	"testing"
)

// TestParseMCPServers covers the env-var parser: empty / valid /
// malformed inputs map to the documented outputs.
func TestParseMCPServers(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []MCPServer
		wantErr bool
	}{
		{
			name: "empty input → nil",
			in:   "",
			want: nil,
		},
		{
			name: "single entry",
			in:   "github=https://mcp.github.com/",
			want: []MCPServer{{Name: "github", Endpoint: "https://mcp.github.com/"}},
		},
		{
			name: "multiple with whitespace",
			in:   " github = https://mcp.github.com/ , lsp = https://mcp.lsp/ ",
			want: []MCPServer{
				{Name: "github", Endpoint: "https://mcp.github.com/"},
				{Name: "lsp", Endpoint: "https://mcp.lsp/"},
			},
		},
		{
			name:    "missing equals",
			in:      "github",
			wantErr: true,
		},
		{
			name:    "trailing equals",
			in:      "github=",
			wantErr: true,
		},
		{
			name:    "empty name",
			in:      "=https://mcp/",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMCPServers(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got=%+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
