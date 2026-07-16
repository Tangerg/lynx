package server

import (
	"context"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/component/mcpstatus"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// fakeMCPPorts implements the four narrow MCP projections consumed by the
// integration use cases. Registry-mutation methods are inert because the
// configuration tests drive a separate durable registry fake.
type fakeMCPPorts struct {
	statuses      []mcpserver.ConnectionStatus
	tools         []mcpserver.ToolInfo
	reconnectName string
	authorizeName string
}

func (f *fakeMCPPorts) Statuses() []mcpserver.ConnectionStatus { return f.statuses }

func (f *fakeMCPPorts) Tools(_ context.Context, server string) ([]mcpserver.ToolInfo, error) {
	if server == "" {
		return f.tools, nil
	}
	var out []mcpserver.ToolInfo
	for _, t := range f.tools {
		if t.Server == server {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeMCPPorts) Reconnect(_ context.Context, name string) error {
	f.reconnectName = name
	return nil
}

func (f *fakeMCPPorts) Authorize(_ context.Context, name string) error {
	f.authorizeName = name
	return nil
}

func (*fakeMCPPorts) Probe(context.Context, mcpserver.LiveConfig) error     { return nil }
func (*fakeMCPPorts) Configure(context.Context, mcpserver.LiveConfig) error { return nil }
func (*fakeMCPPorts) Remove(context.Context, string)                        {}

func fakeMCPPortsConfig(ports *fakeMCPPorts) integrations.Config {
	return integrations.Config{
		MCPStatusReader:       ports,
		MCPToolCatalog:        ports,
		MCPConnectionCommands: ports,
		MCPRegistryCommands:   ports,
	}
}

// mcpRegistryFake is the mcpserver.Registry the MCP config handlers drive.
type mcpRegistryFake struct {
	servers    map[string]mcpserver.Server
	getErr     error
	configured []mcpserver.Server
}

func (r *mcpRegistryFake) List(context.Context) ([]mcpserver.Server, error) {
	out := make([]mcpserver.Server, 0, len(r.servers))
	for _, srv := range r.servers {
		out = append(out, srv)
	}
	return out, nil
}

func (r *mcpRegistryFake) Get(_ context.Context, name string) (mcpserver.Server, bool, error) {
	if r.getErr != nil {
		return mcpserver.Server{}, false, r.getErr
	}
	srv, ok := r.servers[name]
	return srv, ok, nil
}

func (r *mcpRegistryFake) Configure(_ context.Context, srv mcpserver.Server) error {
	r.configured = append(r.configured, srv)
	return nil
}

func (*mcpRegistryFake) Remove(context.Context, string) error           { return nil }
func (*mcpRegistryFake) SetEnabled(context.Context, string, bool) error { return nil }

// serverWithMCP builds a Server whose capabilities coordinator is wired for the
// MCP handlers (live pool + registry + policy), plus the workspace event hub the
// reconnect/configure paths publish through — bridged like the composition root
// via an mcpstatus.Notifier so the coordinator's connecting → settled frames
// reach the hub.
func serverWithMCP(cfg integrations.Config) *Server {
	if cfg.MCPPolicy == nil {
		cell := &atomic.Pointer[mcpserver.ToolPolicy]{}
		policy := mcpserver.NewToolPolicy(nil)
		cell.Store(&policy)
		cfg.MCPPolicy = cell
	}
	mcpStatus := &mcpstatus.Notifier{}
	cfg.MCPStatus = mcpStatus.Publish
	s := &Server{integrations: integrations.New(cfg), wsHub: newWorkspaceHub()}
	s.observeMCPStatus(mcpStatus)
	return s
}
