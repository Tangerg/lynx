package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

func TestRuntimeMCPLiveStatusAndToolsUsePorts(t *testing.T) {
	live := &fakeMCPLive{
		statuses: []toolport.MCPServerStatus{{Name: "fs", Status: "connected"}},
		tools:    []toolport.MCPToolInfo{{Server: "fs", Name: "read"}},
	}
	rt := &Runtime{
		mcpLiveStatus: live,
		mcpLiveTools:  live,
	}

	if got := rt.MCPServerStatuses(); len(got) != 1 || got[0].Name != "fs" {
		t.Fatalf("MCPServerStatuses = %+v", got)
	}
	tools, err := rt.MCPTools(context.Background(), "fs")
	if err != nil {
		t.Fatalf("MCPTools err = %v", err)
	}
	if live.toolsServer != "fs" || len(tools) != 1 || tools[0].Name != "read" {
		t.Fatalf("tools server=%q tools=%+v", live.toolsServer, tools)
	}
}

func TestRuntimeMCPLiveConnectionCommandsUsePort(t *testing.T) {
	live := &fakeMCPLive{}
	rt := &Runtime{mcpLiveConnections: live}

	if err := rt.ReconnectMCPServer(context.Background(), "fs"); err != nil {
		t.Fatalf("ReconnectMCPServer err = %v", err)
	}
	if err := rt.AuthorizeMCPServer(context.Background(), "github"); err != nil {
		t.Fatalf("AuthorizeMCPServer err = %v", err)
	}

	if live.reconnectName != "fs" || live.authorizeName != "github" {
		t.Fatalf("reconnect=%q authorize=%q", live.reconnectName, live.authorizeName)
	}
}

func TestRuntimeTestMCPServerUsesLiveRegistryPort(t *testing.T) {
	live := &fakeMCPLive{}
	rt := &Runtime{mcpLiveRegistry: live}

	err := rt.TestMCPServer(context.Background(), mcpserver.Server{
		Name:      "fs",
		Transport: mcpserver.TransportStdio,
		Command:   "mcp-fs",
		Args:      []string{"--root", "/repo"},
		Env:       map[string]string{"A": "1"},
	})
	if err != nil {
		t.Fatalf("TestMCPServer err = %v", err)
	}
	if live.probe.Name != "fs" || live.probe.Command != "mcp-fs" || len(live.probe.Env) != 1 || live.probe.Env[0] != "A=1" {
		t.Fatalf("probe config = %+v", live.probe)
	}
}

type fakeMCPLive struct {
	statuses []toolport.MCPServerStatus
	tools    []toolport.MCPToolInfo

	toolsServer string

	reconnectName string
	authorizeName string

	probe      toolport.MCPServerConfig
	configure  toolport.MCPServerConfig
	removeName string
}

func (f *fakeMCPLive) MCPServerStatuses() []toolport.MCPServerStatus {
	return f.statuses
}

func (f *fakeMCPLive) MCPTools(_ context.Context, server string) ([]toolport.MCPToolInfo, error) {
	f.toolsServer = server
	return f.tools, nil
}

func (f *fakeMCPLive) ReconnectMCPServer(_ context.Context, name string) error {
	f.reconnectName = name
	return nil
}

func (f *fakeMCPLive) AuthorizeMCPServer(_ context.Context, name string) error {
	f.authorizeName = name
	return nil
}

func (f *fakeMCPLive) ProbeMCPServer(_ context.Context, cfg toolport.MCPServerConfig) error {
	f.probe = cfg
	return nil
}

func (f *fakeMCPLive) ConfigureMCPServer(_ context.Context, cfg toolport.MCPServerConfig) error {
	f.configure = cfg
	return nil
}

func (f *fakeMCPLive) RemoveMCPServer(_ context.Context, name string) {
	f.removeName = name
}
