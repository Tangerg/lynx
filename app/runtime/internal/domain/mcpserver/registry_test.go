package mcpserver

import (
	"testing"
	"time"
)

func TestServerValidateRejectsNegativeTimeout(t *testing.T) {
	srv := Server{
		Name:      "linear",
		Transport: TransportStreamableHTTP,
		URL:       "https://mcp.linear.app/mcp",
		Timeout:   -time.Second,
	}

	if err := srv.Validate(); err == nil {
		t.Fatal("Validate err = nil, want negative timeout rejected")
	}
}

func TestServerValidateRejectsCrossTransportState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Server)
	}{
		{name: "command", mutate: func(server *Server) { server.Command = "node" }},
		{name: "args", mutate: func(server *Server) { server.Args = []string{"server.js"} }},
		{name: "environment", mutate: func(server *Server) { server.Env = map[string]string{"TOKEN": "secret"} }},
		{name: "working directory", mutate: func(server *Server) { server.Dir = "/repo" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := Server{
				Name: "cloud", Transport: TransportStreamableHTTP, URL: "https://example.com/mcp",
			}
			test.mutate(&server)
			if err := server.Validate(); err == nil {
				t.Fatal("Validate err = nil, want cross-transport state rejected")
			}
		})
	}
}
