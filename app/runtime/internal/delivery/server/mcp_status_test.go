package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestListMCPServers(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{
			{Name: "fs", State: mcpserver.ConnectionConnected},
			{Name: "down", State: mcpserver.ConnectionFailed},
		},
		tools: []mcpserver.ToolInfo{
			{Server: "fs", Name: "read"}, {Server: "fs", Name: "write"},
		},
	}))
	page, err := s.ListMCPServers(context.Background(), protocol.PageQuery{})
	if err != nil {
		t.Fatalf("listServers: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("servers = %+v, want 2 (connected + failed)", page.Data)
	}
	fs := page.Data[0]
	if fs.Status != "connected" || fs.ToolCount == nil || *fs.ToolCount != 2 || fs.Error != nil {
		t.Fatalf("fs = %+v, want connected toolCount=2 no error", fs)
	}
	down := page.Data[1]
	if down.Status != "failed" || down.ToolCount != nil || down.Error == nil || down.Error.Detail != "MCP connection failed." {
		t.Fatalf("down = %+v, want failed + safe error no toolCount", down)
	}
}

func TestMCPStateWire(t *testing.T) {
	tests := []struct {
		name  string
		state mcpserver.ConnectionState
		want  protocol.McpStatus
		ok    bool
	}{
		{"connecting", mcpserver.ConnectionConnecting, protocol.McpConnecting, true},
		{"connected", mcpserver.ConnectionConnected, protocol.McpConnected, true},
		{"failed", mcpserver.ConnectionFailed, protocol.McpFailed, true},
		{"needs auth", mcpserver.ConnectionNeedsAuth, protocol.McpNeedsAuth, true},
		{"unknown", mcpserver.ConnectionState("typo"), "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mcpStateWire(tt.state)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("mcpStateWire(%q) = (%q, %t), want (%q, %t)", tt.state, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestMCPServerWireRejectsUnknownDomainState(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{}))
	got := s.mcpServerWire(integrations.MCPServerStatus{
		Name: "broken", Known: true, State: mcpserver.ConnectionState("typo"),
	})
	if got.Status != protocol.McpFailed || got.Error == nil || got.Error.Type != "mcp_invalid_connection_state" {
		t.Fatalf("mcpServerWire(unknown state) = %+v, want explicit failed projection", got)
	}
}

func TestReconnectMCPServer(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected}},
		tools:    []mcpserver.ToolInfo{{Server: "fs", Name: "read"}},
	}))
	defer s.Close()
	events, unsub := s.wsHub.subscribe()
	defer unsub()

	if err := s.ReconnectMCPServer(context.Background(), "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if first := <-events; first.Type != "mcp.serverChanged" || first.Server != "fs" || first.Status != "connecting" {
		t.Fatalf("first event = %+v, want fs connecting", first)
	}
	if term := <-events; term.Status != "connected" || term.ToolCount == nil || *term.ToolCount != 1 {
		t.Fatalf("terminal event = %+v, want fs connected toolCount=1", term)
	}

	if err := s.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("reconnect unknown = %v, want ErrInvalidParams", err)
	}
}

func TestListMCPTools(t *testing.T) {
	readSchema, err := mcpserver.ParseInputSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("ParseInputSchema: %v", err)
	}
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{tools: []mcpserver.ToolInfo{
		{Server: "fs", Name: "read", Description: "read a file", InputSchema: readSchema},
		{Server: "fs", Name: "write"},
		{Server: "git", Name: "log"},
	}}))

	all, err := s.ListMCPTools(context.Background(), protocol.MCPListToolsRequest{})
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(all.Data) != 3 || all.Data[0].Server != "fs" || all.Data[0].Name != "read" || all.Data[0].InputSchema["type"] != "object" {
		t.Fatalf("all = %+v, want 3 with fs/read carrying its schema", all.Data)
	}

	scoped, err := s.ListMCPTools(context.Background(), protocol.MCPListToolsRequest{Server: "git"})
	if err != nil {
		t.Fatalf("listTools(git): %v", err)
	}
	if len(scoped.Data) != 1 || scoped.Data[0].Server != "git" {
		t.Fatalf("scoped = %+v, want only git tools", scoped.Data)
	}
}
