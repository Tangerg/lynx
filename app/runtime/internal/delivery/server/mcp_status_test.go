package server

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestMCPAuthorizationAttemptWire(t *testing.T) {
	finishedAt := time.Date(2026, 8, 2, 12, 1, 0, 0, time.UTC)
	got := presentMCPAuthorizationAttempt(mcpapp.AuthorizationAttempt{
		ID: "mcpauth_example", Server: "github",
		Status:    mcpapp.AuthorizationAttemptFailed,
		CreatedAt: finishedAt.Add(-time.Minute), FinishedAt: &finishedAt,
	})
	if got.Status.Type != protocol.MCPAuthorizationAttemptFailed || got.Status.Error == nil ||
		got.Status.Error.Type != protocol.ProblemMCPAuthorizationFailed || got.Status.Error.Detail != "" ||
		got.FinishedAt == nil {
		t.Fatalf("authorization attempt = %+v", got)
	}
	if err := protocol.ValidateWireTree(got); err != nil {
		t.Fatalf("authorization attempt violates wire contract: %v", err)
	}
}

func TestListMCPServers(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{
			{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 2},
			{Name: "down", State: mcpserver.ConnectionFailed},
		},
		tools: []mcpserver.AdvertisedTool{
			{Server: "fs", Name: "read"}, {Server: "fs", Name: "write"},
		},
	}))
	page, err := s.ListMCPServers(context.Background())
	if err != nil {
		t.Fatalf("listServers: %v", err)
	}
	if len(page.Data) != 2 {
		t.Fatalf("servers = %+v, want 2 (connected + failed)", page.Data)
	}
	byName := make(map[string]protocol.MCPServer, len(page.Data))
	for _, server := range page.Data {
		byName[server.Name] = server
	}
	fs := byName["fs"]
	if fs.Status.Type != protocol.MCPServerConnected || fs.Status.ToolCount == nil || *fs.Status.ToolCount != 2 || fs.Status.Error != nil {
		t.Fatalf("fs = %+v, want connected toolCount=2 no error", fs)
	}
	down := byName["down"]
	if down.Status.Type != protocol.MCPServerFailed || down.Status.ToolCount != nil || down.Status.Error == nil ||
		down.Status.Error.Type != protocol.ProblemMCPDialFailed || down.Status.Error.Detail != "" {
		t.Fatalf("down = %+v, want failed + the dial-failed symbol and no prose", down)
	}
}

func TestMCPServerStateWire(t *testing.T) {
	tests := []struct {
		name  string
		state mcpapp.ServerState
		want  protocol.MCPServerStateType
	}{
		{"disabled", mcpapp.ServerState{Type: mcpapp.ServerDisabled}, protocol.MCPServerDisabled},
		{"disconnected", mcpapp.ServerState{Type: mcpapp.ServerDisconnected}, protocol.MCPServerDisconnected},
		{"connecting", mcpapp.ServerState{Type: mcpapp.ServerConnecting}, protocol.MCPServerConnecting},
		{"connected", mcpapp.ServerState{Type: mcpapp.ServerConnected}, protocol.MCPServerConnected},
		{"failed", mcpapp.ServerState{Type: mcpapp.ServerFailed}, protocol.MCPServerFailed},
		{"needs auth", mcpapp.ServerState{Type: mcpapp.ServerNeedsAuth}, protocol.MCPServerNeedsAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := presentMCPServerState(tt.state).Type; got != tt.want {
				t.Fatalf("presentMCPServerState(%v) = %q, want %q", tt.state.Type, got, tt.want)
			}
		})
	}
}

// A connection state this projection does not map is a defect in the projection,
// not a state a server can be in. Answering with a failed server and an invented
// problem type shipped that defect as a verdict a user could read and act on.
func TestMCPServerWireRejectsUnknownDomainState(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("presentMCPServer(unknown state) answered with a projection instead of panicking")
		}
	}()
	_, _ = presentMCPServer(mcpapp.Server{
		Name:       "broken",
		Connection: mcpapp.Connection{Transport: mcpserver.TransportStdio, Command: "broken"},
		State:      mcpapp.ServerState{Type: mcpapp.ServerStateType(255)},
	})
}

func TestReconnectMCPServer(t *testing.T) {
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{
		statuses: []mcpserver.ConnectionStatus{{Name: "fs", State: mcpserver.ConnectionConnected, ToolCount: 1}},
		tools:    []mcpserver.AdvertisedTool{{Server: "fs", Name: "read"}},
	}))
	defer s.Close()
	events, unsub := s.workspaceHub.subscribe()
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

	if err := s.ReconnectMCPServer(context.Background(), "ghost"); !errors.Is(err, protocol.ErrMCPServerNotFound) {
		t.Fatalf("reconnect unknown = %v, want ErrMCPServerNotFound", err)
	}
}

func TestListMCPTools(t *testing.T) {
	readSchema, err := mcpserver.ParseInputSchema([]byte(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("ParseInputSchema: %v", err)
	}
	s := serverWithMCP(fakeMCPPortsConfig(&fakeMCPPorts{tools: []mcpserver.AdvertisedTool{
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
