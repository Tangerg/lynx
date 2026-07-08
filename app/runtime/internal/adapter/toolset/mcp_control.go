package toolset

import (
	"context"
	"errors"
	"fmt"

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

func (c *mcpControl) Statuses() []toolport.MCPServerStatus {
	if c == nil || c.inner == nil {
		return nil
	}
	statuses := c.inner.Statuses()
	out := make([]toolport.MCPServerStatus, len(statuses))
	for i, st := range statuses {
		out[i] = toolport.MCPServerStatus{
			Name:   st.Name,
			Status: st.Status,
			Err:    st.Err,
		}
	}
	return out
}

func (c *mcpControl) Tools(ctx context.Context, server string) ([]toolport.MCPToolInfo, error) {
	tools, err := c.inner.Tools(ctx, server)
	if err != nil {
		return nil, mapMCPError(err)
	}
	out := make([]toolport.MCPToolInfo, len(tools))
	for i, t := range tools {
		out[i] = toolport.MCPToolInfo{
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

func (c *mcpControl) Probe(ctx context.Context, cfg toolport.MCPServerConfig) error {
	return mapMCPError(c.inner.Probe(ctx, infraMCPServerConfig(cfg)))
}

func (c *mcpControl) Configure(ctx context.Context, cfg toolport.MCPServerConfig) error {
	return mapMCPError(c.inner.Configure(ctx, infraMCPServerConfig(cfg)))
}

func (c *mcpControl) Remove(ctx context.Context, name string) {
	c.inner.Remove(ctx, name)
}

func mapMCPError(err error) error {
	if errors.Is(err, mcp.ErrUnknownServer) {
		return fmt.Errorf("%w: %w", toolport.ErrUnknownMCPServer, err)
	}
	return err
}
