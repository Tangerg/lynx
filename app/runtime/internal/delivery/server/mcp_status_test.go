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
			{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 2},
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
	if down.Status != "failed" || down.ToolCount != nil || down.Error == nil ||
		down.Error.Type != protocol.ProblemMCPDialFailed || down.Error.Detail != "" {
		t.Fatalf("down = %+v, want failed + the dial-failed symbol and no prose", down)
	}
}

func TestMCPStateWire(t *testing.T) {
	tests := []struct {
		name  string
		state mcpserver.ConnectionState
		want  protocol.McpStatus
	}{
		{"connecting", mcpserver.ConnectionConnecting, protocol.McpConnecting},
		{"connected", mcpserver.ConnectionConnected, protocol.McpConnected},
		{"failed", mcpserver.ConnectionFailed, protocol.McpFailed},
		{"needs auth", mcpserver.ConnectionNeedsAuth, protocol.McpNeedsAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpStateWire(tt.state); got != tt.want {
				t.Fatalf("mcpStateWire(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// A connection state this projection does not map is a defect in the projection,
// not a state a server can be in. Answering with a failed server and an invented
// problem type shipped that defect as a verdict a user could read and act on.
func TestMCPServerWireRejectsUnknownDomainState(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{}))
	defer func() {
		if recover() == nil {
			t.Fatal("mcpServerWire(unknown state) answered with a projection instead of panicking")
		}
	}()
	s.mcpServerWire(integrations.MCPServerStatus{
		Name: "broken", Known: true, State: mcpserver.ConnectionState("typo"),
	})
}

func TestReconnectMCPServer(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 1}},
		tools:    []mcpserver.ToolInfo{{Server: "fs", Name: "read"}},
	}))
	defer s.Close()
	events, unsub := s.wsHub.subscribe()
	defer unsub()

	if err := s.ReconnectMCPServer(context.Background(), "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	// Both transitions are the same signal now: "this server moved, read it again".
	// The status and tool count used to ride the frame, which made the stream a second
	// answer to what mcp.servers already knows.
	for _, phase := range []string{"connecting", "settled"} {
		event := <-events
		if event.Type != protocol.RuntimeMCPChanged || len(event.ServerIDs) != 1 || event.ServerIDs[0] != "fs" {
			t.Fatalf("%s event = %+v, want the fs change signal", phase, event)
		}
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
