package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

func TestWorkspaceMCPListServers(t *testing.T) {
	s := newTestServer(&stubRuntime{
		mcpStatuses: []toolport.MCPServerStatus{
			{Name: "fs", Status: "connected"},
			{Name: "down", Status: "failed", Err: errors.New("connection refused")},
		},
		mcpTools: []toolport.MCPToolInfo{
			{Server: "fs", Name: "read"}, {Server: "fs", Name: "write"},
		},
	})
	page, err := s.WorkspaceMCPListServers(context.Background(), protocol.PageQuery{})
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
	if down.Status != "failed" || down.ToolCount != nil || down.Error == nil || down.Error.Detail != "connection refused" {
		t.Fatalf("down = %+v, want failed + Error(connection refused) no toolCount", down)
	}
}

func TestWorkspaceMCPReconnect(t *testing.T) {
	s := &Server{
		rt: &stubRuntime{
			mcpStatuses: []toolport.MCPServerStatus{{Name: "fs", Status: "connected"}},
			mcpTools:    []toolport.MCPToolInfo{{Server: "fs", Name: "read"}},
		},
		wsHub: newWorkspaceHub(),
	}
	defer s.Close()
	events, unsub := s.wsHub.subscribe()
	defer unsub()

	if err := s.WorkspaceMCPReconnect(context.Background(), "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if first := <-events; first.Type != "mcp.serverChanged" || first.Server != "fs" || first.Status != "connecting" {
		t.Fatalf("first event = %+v, want fs connecting", first)
	}
	if term := <-events; term.Status != "connected" || term.ToolCount == nil || *term.ToolCount != 1 {
		t.Fatalf("terminal event = %+v, want fs connected toolCount=1", term)
	}

	if err := s.WorkspaceMCPReconnect(context.Background(), "ghost"); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("reconnect unknown = %v, want ErrInvalidParams", err)
	}
}

func TestMCPConnectionActionIsDetachedButServerOwned(t *testing.T) {
	type contextKey struct{}
	s := &Server{
		rt: &stubRuntime{mcpStatuses: []toolport.MCPServerStatus{{Name: "fs", Status: "connected"}}},
	}
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "trace"))
	cancelRequest()

	started := make(chan bool, 1)
	stopped := make(chan struct{})
	err := s.runMCPConnectionAction(requestCtx, "fs", func(ctx context.Context, _ string) error {
		started <- ctx.Err() == nil && ctx.Value(contextKey{}) == "trace"
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	if err != nil {
		t.Fatalf("run action: %v", err)
	}
	if detached := <-started; !detached {
		t.Fatal("action context did not detach request cancellation or preserve values")
	}

	s.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Server.Close did not cancel and join the action")
	}
	if err := s.runMCPConnectionAction(context.Background(), "fs", func(context.Context, string) error { return nil }); !errors.Is(err, errServerClosed) {
		t.Fatalf("action after Close = %v, want errServerClosed", err)
	}
}

func TestWorkspaceMCPListTools(t *testing.T) {
	s := newTestServer(&stubRuntime{mcpTools: []toolport.MCPToolInfo{
		{Server: "fs", Name: "read", Description: "read a file", InputSchema: map[string]any{"type": "object"}},
		{Server: "fs", Name: "write"},
		{Server: "git", Name: "log"},
	}})

	all, err := s.WorkspaceMCPListTools(context.Background(), protocol.MCPListToolsRequest{})
	if err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(all.Data) != 3 || all.Data[0].Server != "fs" || all.Data[0].Name != "read" || all.Data[0].InputSchema["type"] != "object" {
		t.Fatalf("all = %+v, want 3 with fs/read carrying its schema", all.Data)
	}

	scoped, err := s.WorkspaceMCPListTools(context.Background(), protocol.MCPListToolsRequest{Server: "git"})
	if err != nil {
		t.Fatalf("listTools(git): %v", err)
	}
	if len(scoped.Data) != 1 || scoped.Data[0].Server != "git" {
		t.Fatalf("scoped = %+v, want only git tools", scoped.Data)
	}
}
