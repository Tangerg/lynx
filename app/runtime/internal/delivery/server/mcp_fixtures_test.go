package server

import (
	"cmp"
	"context"
	"slices"
	"sync"

	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
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

func (*fakeMCPPorts) Probe(context.Context, mcpserver.Server) error     { return nil }
func (*fakeMCPPorts) Configure(context.Context, mcpserver.Server) error { return nil }
func (*fakeMCPPorts) Detach(string) error                               { return nil }

func fakeMCPPortsConfig(ports *fakeMCPPorts) mcpapp.Config {
	servers := make(map[string]mcpserver.Server, len(ports.statuses))
	for _, status := range ports.statuses {
		servers[status.Name] = mcpserver.Server{
			Name: status.Name, Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-" + status.Name,
		}
	}
	return mcpapp.Config{
		Registry:            &mcpRegistryFake{servers: servers},
		StatusReader:        ports,
		ToolCatalog:         ports,
		ConnectionControl:   ports,
		ConnectionLifecycle: ports,
	}
}

// mcpRegistryFake is the integration registry the MCP config handlers drive.
type mcpRegistryFake struct {
	mu      sync.Mutex
	servers map[string]mcpserver.Server
	getErr  error
	saved   []mcpserver.Server
}

func (r *mcpRegistryFake) List(context.Context) ([]mcpserver.Server, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mcpserver.Server, 0, len(r.servers))
	for _, srv := range r.servers {
		out = append(out, srv)
	}
	slices.SortFunc(out, func(a, b mcpserver.Server) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return out, nil
}

func (r *mcpRegistryFake) Get(_ context.Context, name string) (mcpserver.Server, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return mcpserver.Server{}, false, r.getErr
	}
	srv, ok := r.servers[name]
	return srv, ok, nil
}

func (r *mcpRegistryFake) Save(_ context.Context, srv mcpserver.Server) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.servers == nil {
		r.servers = make(map[string]mcpserver.Server)
	}
	r.servers[srv.Name] = srv
	r.saved = append(r.saved, srv)
	return nil
}

func (r *mcpRegistryFake) Remove(_ context.Context, name string) error {
	r.mu.Lock()
	delete(r.servers, name)
	r.mu.Unlock()
	return nil
}

// serverWithMCP builds a Server whose capabilities coordinator is wired for the
// MCP handlers (live pool + registry + policy), plus the workspace event hub the
// reconnect/configure paths publish through — bridged like the composition root
// via a neutral signal so the coordinator's connecting → settled frames
// reach the hub.
func serverWithMCP(cfg mcpapp.Config) *Server {
	if cfg.Policy == nil {
		policy := mcpserver.NewToolPolicy(nil)
		cfg.Policy = mcpapp.NewToolPolicyState(policy)
	}
	mcpStatus := &testNotification[mcpapp.ServerStatus]{}
	cfg.StatusChanged = mcpStatus.Publish
	s := &Server{mcp: mcpapp.New(cfg), wsHub: newWorkspaceHub()}
	s.observeMCPStatusChanges(mcpStatus)
	return s
}
