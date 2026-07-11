package toolset

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/toolport"
)

type mcpControl struct {
	inner *mcp.Connections
}

var (
	_ toolport.MCPStatusReader       = (*mcpControl)(nil)
	_ toolport.MCPToolCatalog        = (*mcpControl)(nil)
	_ toolport.MCPConnectionCommands = (*mcpControl)(nil)
	_ toolport.MCPRegistryCommands   = (*mcpControl)(nil)
)

func (c *mcpControl) Statuses() []mcpserver.ConnectionStatus {
	if c == nil || c.inner == nil {
		return nil
	}
	statuses := c.inner.Statuses()
	out := make([]mcpserver.ConnectionStatus, len(statuses))
	for i, st := range statuses {
		out[i] = mcpserver.ConnectionStatus{
			Name:   st.Name,
			Status: st.Status,
			Err:    st.Err,
		}
	}
	return out
}

func (c *mcpControl) Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
	tools, err := c.inner.Tools(ctx, server)
	if err != nil {
		return nil, mapMCPError(err)
	}
	out := make([]mcpserver.ToolInfo, len(tools))
	for i, t := range tools {
		out[i] = mcpserver.ToolInfo{
			Server:      t.Server,
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return out, nil
}

func (c *mcpControl) Reconnect(ctx context.Context, name string) error {
	return mapMCPError(c.inner.Reconnect(ctx, name))
}

func (c *mcpControl) Authorize(ctx context.Context, name string) error {
	return mapMCPError(c.inner.Authorize(ctx, name))
}

func (c *mcpControl) Probe(ctx context.Context, cfg mcpserver.LiveConfig) error {
	return mapMCPError(c.inner.Probe(ctx, infraMCPServerConfig(cfg)))
}

func (c *mcpControl) Configure(ctx context.Context, cfg mcpserver.LiveConfig) error {
	return mapMCPError(c.inner.Configure(ctx, infraMCPServerConfig(cfg)))
}

func (c *mcpControl) Remove(ctx context.Context, name string) {
	c.inner.Remove(ctx, name)
}

func mapMCPError(err error) error {
	if errors.Is(err, mcp.ErrUnknownServer) {
		return fmt.Errorf("%w: %w", mcpserver.ErrUnknownServer, err)
	}
	return err
}
