package integrations

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ListMCPServerConfigs returns safe editable MCP configurations. The durable
// domain entries never cross the application boundary because they carry raw
// authorization tokens.
func (c *Coordinator) ListMCPServerConfigs(ctx context.Context) ([]MCPServerConfig, error) {
	if c.mcpRegistry == nil {
		return nil, errors.New("integrations: MCP registry is unavailable")
	}
	servers, err := c.mcpRegistry.List(ctx)
	if err != nil {
		return nil, err
	}
	configs := make([]MCPServerConfig, 0, len(servers))
	for _, server := range servers {
		configs = append(configs, mcpConfigView(server))
	}
	return configs, nil
}

// MCPServerStatuses resolves the safe live status read model for every tracked
// server. Raw adapter errors are intentionally reduced to stable diagnostics.
func (c *Coordinator) MCPServerStatuses(ctx context.Context) []MCPServerStatus {
	if c.mcpStatusReader == nil {
		return nil
	}
	statuses := c.mcpStatusReader.Statuses()
	views := make([]MCPServerStatus, 0, len(statuses))
	for _, status := range statuses {
		views = append(views, c.mcpStatusView(ctx, status))
	}
	return views
}

// MCPServerStatus resolves one safe live status read model.
func (c *Coordinator) MCPServerStatus(ctx context.Context, name string) MCPServerStatus {
	if c.mcpStatusReader == nil {
		return MCPServerStatus{Name: name}
	}
	for _, status := range c.mcpStatusReader.Statuses() {
		if status.Name == name {
			return c.mcpStatusView(ctx, status)
		}
	}
	return MCPServerStatus{Name: name}
}

func (c *Coordinator) mcpStatusView(ctx context.Context, status mcpserver.ConnectionStatus) MCPServerStatus {
	var toolCount *int
	if status.State == mcpserver.ConnectionConnected && c.mcpToolCatalog != nil {
		if tools, err := c.mcpToolCatalog.Tools(ctx, status.Name); err == nil {
			count := len(tools)
			toolCount = &count
		}
	}
	return mcpStatusView(status, toolCount)
}
