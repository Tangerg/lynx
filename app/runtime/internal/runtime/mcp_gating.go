package runtime

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// mcpGating is the per-call MCP tool gating derived from the registry's enabled
// servers, both keyed on the model-facing name published by the MCP tool port:
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
	configs     []kernel.MCPServerConfig
}

func buildMCPEnvironment(ctx context.Context, registry mcpServerList) (mcpEnvironment, error) {
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
func buildMCPGating(ctx context.Context, svc mcpServerList) (*mcpGating, error) {
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
			g.disabled[mcpserver.ToolName(s.Name, tool)] = struct{}{}
		}
		for _, tool := range s.AutoApproveTools {
			g.autoApprove[mcpserver.ToolName(s.Name, tool)] = struct{}{}
		}
	}
	return g, nil
}

// refreshMCPGating recomputes the gating sets from the (just-mutated) registry
// and swaps them in atomically, so a configure/remove/enable takes effect for
// the next tool resolution and the next approval gate without a restart.
func (r *Runtime) refreshMCPGating(ctx context.Context) error {
	g, err := buildMCPGating(ctx, r.mcpRegistryList)
	if err != nil {
		return err
	}
	r.mcpGating.Store(g)
	return nil
}
