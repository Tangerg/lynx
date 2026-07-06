package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// MCP-server registry orchestration: the runtime owns both the persisted
// registry (mcpserver.Service) and the live connections (via the engine's
// MCPControl facade), so a configure/remove/enable both persists and applies
// to the live tool set in one place. The dial-level descriptor (mcp.ServerConfig)
// and the registry entry (mcpserver.Server) are bridged by the converters here.

// ListMCPRegisteredServers returns the persisted MCP-server registry entries,
// distinct from the live connection statuses returned by MCPServerStatuses.
func (r *Runtime) ListMCPRegisteredServers(ctx context.Context) ([]mcpserver.Server, error) {
	return r.mcpRegistry.List(ctx)
}

// MCPServerStatuses returns the per-server connection state of every
// configured MCP server (connected and boot-failed alike) for
// workspace.mcp.listServers. Delegates to the engine, which owns the sessions.
func (r *Runtime) MCPServerStatuses() []kernel.McpServerStatus {
	return r.engine.MCPServerStatuses()
}

// MCPRegisteredServer returns one persisted MCP-server registry entry.
func (r *Runtime) MCPRegisteredServer(ctx context.Context, name string) (mcpserver.Server, bool, error) {
	return r.mcpRegistry.Get(ctx, name)
}

// ReconnectMCPServer re-dials a configured MCP server and hot-swaps the live
// tool set (workspace.mcp.reconnect). Delegates to the engine, which owns the
// sessions + the shared client.
func (r *Runtime) ReconnectMCPServer(ctx context.Context, name string) error {
	return r.engine.ReconnectMCPServer(ctx, name)
}

// AuthorizeMCPServer runs the interactive OAuth sign-in for an HTTP MCP server
// (workspace.mcp.authorize) — opens the system browser, catches the loopback
// redirect, and connects on success. Delegates to the engine, which owns the
// sessions. The credentials live for the process only (re-prompt after restart).
func (r *Runtime) AuthorizeMCPServer(ctx context.Context, name string) error {
	return r.engine.AuthorizeMCPServer(ctx, name)
}

// ConfigureMCPServer upserts a server in the registry and applies it to the
// live connections: an enabled server is (re)dialed, a disabled one is dropped
// from the live set (it stays in the registry). A dial failure does not fail
// the call — the server is persisted and tracked "failed" (reconnectable); the
// connectivity feedback path is TestMCPServer.
func (r *Runtime) ConfigureMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	if err := r.mcpRegistry.Configure(ctx, srv); err != nil {
		return err
	}
	return r.applyAndGate(ctx, srv)
}

// RemoveMCPServer deletes a server from the registry and drops it from the live
// connections.
func (r *Runtime) RemoveMCPServer(ctx context.Context, name string) error {
	if err := r.mcpRegistry.Remove(ctx, name); err != nil {
		return err
	}
	// Shrink the live set before the gating (the disable direction of the
	// applyAndGate rule): dropping tools can't expose a hidden one, but
	// shrinking the gating first would leave the about-to-be-dropped tools
	// briefly live and ungated.
	r.engine.RemoveMCPServer(ctx, name)
	return r.refreshMCPGating(ctx)
}

// SetMCPServerEnabled flips a server's enablement in the registry and applies
// it to the live connections (enable → dial, disable → drop).
func (r *Runtime) SetMCPServerEnabled(ctx context.Context, name string, enabled bool) error {
	if err := r.mcpRegistry.SetEnabled(ctx, name, enabled); err != nil {
		return err
	}
	srv, ok, err := r.mcpRegistry.Get(ctx, name)
	if err != nil || !ok {
		return err
	}
	return r.applyAndGate(ctx, srv)
}

// applyAndGate reflects a just-persisted registry entry into BOTH the live tool
// set (engine) and the gating sets (atomic cell), ordered so a tool that should
// be hidden is never momentarily visible to the model. The two are read together
// at tool-resolution time but published here in two steps, so the order matters
// by direction:
//   - enabling: the server's tools are about to APPEAR, so the gating that hides
//     some of them must publish first (refresh → apply);
//   - disabling: the tools are about to be DROPPED, so the live set shrinks first,
//     then the gating (apply → refresh).
//
// Either reversal would leave a window where a disabled tool is live but ungated.
// The caller has already mutated the registry, so refreshMCPGating reads the new
// gating lists.
func (r *Runtime) applyAndGate(ctx context.Context, srv mcpserver.Server) error {
	if srv.Enabled {
		if err := r.refreshMCPGating(ctx); err != nil {
			return err
		}
		r.applyMCPServer(ctx, srv)
		return nil
	}
	r.applyMCPServer(ctx, srv)
	return r.refreshMCPGating(ctx)
}

// TestMCPServer dials srv with a throwaway client and proves its tools list —
// a connection test that touches neither the registry nor the live set, EXCEPT
// it reuses an active OAuth sign-in for the same-named server (so an authorized
// OAuth server tests as connected, not "unauthorized"). Returns the dial /
// tools-list error, or nil on success.
func (r *Runtime) TestMCPServer(ctx context.Context, srv mcpserver.Server) error {
	if err := srv.Validate(); err != nil {
		return err
	}
	return r.engine.ProbeMCPServer(ctx, configFromServer(srv))
}

// MCPTools lists tools advertised by the connected MCP servers (scoped to
// server when non-empty) for workspace.mcp.listTools. Delegates to the
// engine, which holds the dialed sessions.
func (r *Runtime) MCPTools(ctx context.Context, server string) ([]kernel.McpToolInfo, error) {
	return r.engine.MCPTools(ctx, server)
}

// applyMCPServer reflects a registry entry into the live connections: enabled →
// (re)dial, disabled → drop. The dial error is intentionally swallowed (status
// surfaces it); see ConfigureMCPServer.
func (r *Runtime) applyMCPServer(ctx context.Context, srv mcpserver.Server) {
	if srv.Enabled {
		_ = r.engine.ConfigureMCPServer(ctx, configFromServer(srv))
		return
	}
	r.engine.RemoveMCPServer(ctx, srv.Name)
}

// enabledConfigs reads the registry and returns the dial descriptors for the
// enabled servers — the boot-time MCP set handed to toolset.Build.
func enabledConfigs(ctx context.Context, svc mcpserver.Service) ([]mcp.ServerConfig, error) {
	servers, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []mcp.ServerConfig
	for _, s := range servers {
		if s.Enabled {
			out = append(out, configFromServer(s))
		}
	}
	return out, nil
}

// SeedMCPServers writes any env-sourced servers (LYRA_MCP_SERVERS) into the
// registry that aren't already present, mirroring seedConfiguredProvider: the
// env is a first-run seed, runtime edits (a persisted configure) win and are
// left untouched.
func SeedMCPServers(ctx context.Context, svc mcpserver.Service, env []mcp.ServerConfig) error {
	for _, cfg := range env {
		if _, ok, err := svc.Get(ctx, cfg.Name); err != nil {
			return err
		} else if ok {
			continue
		}
		if err := svc.Configure(ctx, serverFromConfig(cfg)); err != nil {
			return err
		}
	}
	return nil
}

// configFromServer maps a registry entry to a dial descriptor. Tool-level
// gating (DisabledTools / AutoApproveTools) is applied at toolset build /
// approval, not at dial, so it has no place here. Env is flattened from the
// registry's KEY→value map to the dial layer's "KEY=value" slice (Go exec's
// native shape).
func configFromServer(s mcpserver.Server) mcp.ServerConfig {
	cfg := mcp.ServerConfig{Name: s.Name, Timeout: s.Timeout}
	switch s.Transport {
	case mcpserver.TransportStreamableHTTP:
		cfg.Transport = mcp.TransportHTTP
		cfg.Endpoint = s.URL
		cfg.Authorization = s.Authorization
		cfg.Headers = s.Headers
	case mcpserver.TransportStdio:
		cfg.Transport = mcp.TransportStdio
		cfg.Command = s.Command
		cfg.Args = s.Args
		cfg.Env = envMapToSlice(s.Env)
		cfg.Dir = s.Dir
	}
	return cfg
}

// mcpGating is the per-call MCP tool gating derived from the registry's enabled
// servers, both keyed on the model-facing qualified name "<server>_<tool>":
// disabled tools are hidden from the model (resolver filter) and auto-approved
// tools skip the approval prompt (turn gate). Held behind an atomic pointer the
// runtime swaps on every registry change; the resolver and the gate read it via
// the closures handed to them at construction. Treated immutable after a store.
type mcpGating struct {
	disabled    map[string]struct{}
	autoApprove map[string]struct{}
}

type mcpEnvironment struct {
	gate        *atomic.Pointer[mcpGating]
	disabled    func() map[string]struct{}
	autoApprove func() map[string]struct{}
	configs     []mcp.ServerConfig
}

func buildMCPEnvironment(ctx context.Context, registry mcpserver.Service) (mcpEnvironment, error) {
	gate := &atomic.Pointer[mcpGating]{}
	g0, err := buildMCPGating(ctx, registry)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("runtime: load mcp gating: %w", err)
	}
	gate.Store(g0)
	configs, err := enabledConfigs(ctx, registry)
	if err != nil {
		return mcpEnvironment{}, fmt.Errorf("runtime: load mcp registry: %w", err)
	}
	return mcpEnvironment{
		gate: gate,
		disabled: func() map[string]struct{} {
			if g := gate.Load(); g != nil {
				return g.disabled
			}
			return nil
		},
		autoApprove: func() map[string]struct{} {
			if g := gate.Load(); g != nil {
				return g.autoApprove
			}
			return nil
		},
		configs: configs,
	}, nil
}

// buildMCPGating reads the registry and projects its ENABLED servers' per-tool
// gating lists into the two qualified-name sets. Disabled servers contribute
// nothing — their tools aren't in the live set anyway.
func buildMCPGating(ctx context.Context, svc mcpserver.Service) (*mcpGating, error) {
	servers, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}
	g := &mcpGating{disabled: map[string]struct{}{}, autoApprove: map[string]struct{}{}}
	for _, s := range servers {
		if !s.Enabled {
			continue
		}
		for _, tool := range s.DisabledTools {
			g.disabled[mcp.QualifiedToolName(s.Name, tool)] = struct{}{}
		}
		for _, tool := range s.AutoApproveTools {
			g.autoApprove[mcp.QualifiedToolName(s.Name, tool)] = struct{}{}
		}
	}
	return g, nil
}

// refreshMCPGating recomputes the gating sets from the (just-mutated) registry
// and swaps them in atomically, so a configure/remove/enable takes effect for
// the next tool resolution and the next approval gate without a restart.
func (r *Runtime) refreshMCPGating(ctx context.Context) error {
	g, err := buildMCPGating(ctx, r.mcpRegistry)
	if err != nil {
		return err
	}
	r.mcpGating.Store(g)
	return nil
}

// serverFromConfig maps an env-sourced dial descriptor to a registry entry
// (enabled, no tool-level gating) for first-run seeding. Env is parsed from the
// dial layer's "KEY=value" slice back to the registry's KEY→value map.
func serverFromConfig(c mcp.ServerConfig) mcpserver.Server {
	s := mcpserver.Server{Name: c.Name, Enabled: true, Timeout: c.Timeout}
	switch c.Transport {
	case mcp.TransportHTTP:
		s.Transport = mcpserver.TransportStreamableHTTP
		s.URL = c.Endpoint
		s.Authorization = c.Authorization
		s.Headers = c.Headers
	case mcp.TransportStdio:
		s.Transport = mcpserver.TransportStdio
		s.Command = c.Command
		s.Args = c.Args
		s.Env = envSliceToMap(c.Env)
		s.Dir = c.Dir
	}
	return s
}

// envMapToSlice flattens a KEY→value map to the "KEY=value" slice exec wants,
// sorted by key so the dialed env is deterministic (stable across restarts and
// in tests). nil/empty yields nil.
func envMapToSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

// envSliceToMap parses "KEY=value" entries back into a map, splitting on the
// FIRST '=' so a value may itself contain '='. An entry with no '=' becomes a
// bare key with an empty value. nil/empty yields nil.
func envSliceToMap(s []string) map[string]string {
	if len(s) == 0 {
		return nil
	}
	m := make(map[string]string, len(s))
	for _, kv := range s {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}
