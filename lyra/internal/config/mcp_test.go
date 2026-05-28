package config

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/engine"
)

// TestParseMCPServers covers the env-var parser across both HTTP
// and stdio transport syntaxes plus error cases.
func TestParseMCPServers(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []engine.MCPServer
		wantErr bool
	}{
		{
			name: "empty input → nil",
			in:   "",
			want: nil,
		},
		{
			name: "single http entry",
			in:   "github=https://mcp.github.com/",
			want: []engine.MCPServer{{
				Name:      "github",
				Transport: engine.MCPTransportHTTP,
				Endpoint:  "https://mcp.github.com/",
			}},
		},
		{
			name: "single stdio entry",
			in:   "fs=stdio:npx -y @modelcontextprotocol/server-filesystem /workspace",
			want: []engine.MCPServer{{
				Name:      "fs",
				Transport: engine.MCPTransportStdio,
				Command:   "npx",
				Args:      []string{"-y", "@modelcontextprotocol/server-filesystem", "/workspace"},
			}},
		},
		{
			name: "stdio with single-word command",
			in:   "time=stdio:mcp-server-time",
			want: []engine.MCPServer{{
				Name:      "time",
				Transport: engine.MCPTransportStdio,
				Command:   "mcp-server-time",
				Args:      []string{},
			}},
		},
		{
			name: "mixed http + stdio with whitespace",
			in:   " github = https://mcp.github.com/ , fs = stdio:npx mcp-server-fs ",
			want: []engine.MCPServer{
				{Name: "github", Transport: engine.MCPTransportHTTP, Endpoint: "https://mcp.github.com/"},
				{Name: "fs", Transport: engine.MCPTransportStdio, Command: "npx", Args: []string{"mcp-server-fs"}},
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
		{
			name:    "unsupported scheme",
			in:      "github=ftp://mcp/",
			wantErr: true,
		},
		{
			name:    "stdio empty command",
			in:      "fs=stdio:",
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
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}
