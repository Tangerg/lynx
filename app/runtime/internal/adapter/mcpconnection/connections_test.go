package mcpconnection

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
)

func TestConfigFromServerMapsEachSupportedTransport(t *testing.T) {
	tests := []struct {
		name string
		in   mcpserver.Server
		want mcp.Transport
	}{
		{
			name: "streamable http",
			in: mcpserver.Server{
				Name: "remote", Transport: mcpserver.TransportStreamableHTTP,
				URL: "https://mcp.example/tools", Authorization: "Bearer token",
				Headers: map[string]string{"X-Trace": "enabled"}, Timeout: time.Second,
			},
			want: mcp.TransportHTTP,
		},
		{
			name: "stdio",
			in: mcpserver.Server{
				Name: "local", Transport: mcpserver.TransportStdio,
				Command: "mcp-server", Args: []string{"--stdio"},
				Env: map[string]string{"B": "two", "A": "one"}, Dir: "/tmp", Timeout: time.Second,
			},
			want: mcp.TransportStdio,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := configFromServer(test.in)
			if err != nil {
				t.Fatalf("configFromServer: %v", err)
			}
			if got.Transport != test.want {
				t.Fatalf("transport = %v, want %v", got.Transport, test.want)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("mapped config invalid: %v", err)
			}
		})
	}
}

func TestConfigFromServerRejectsInvalidDomainValue(t *testing.T) {
	_, err := configFromServer(mcpserver.Server{
		Name: "broken", Transport: mcpserver.Transport("websocket"), URL: "https://mcp.example",
	})
	if err == nil {
		t.Fatal("configFromServer error = nil, want invalid transport")
	}
}
