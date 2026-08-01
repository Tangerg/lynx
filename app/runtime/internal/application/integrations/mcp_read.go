package integrations

import (
	"context"
	"errors"
)

// MCPServers returns every durable MCP server enriched with the current live
// status snapshot. The registry determines membership; the live pool is only a
// projection and therefore cannot make a configured or disabled server vanish.
func (c *Coordinator) MCPServers(ctx context.Context) ([]MCPServer, error) {
	if c.mcpRegistry == nil {
		return nil, errors.New("integrations: MCP registry is unavailable")
	}
	servers, err := c.mcpRegistry.List(ctx)
	if err != nil {
		return nil, err
	}
	statuses := c.mcpStatusesByName()
	out := make([]MCPServer, 0, len(servers))
	for _, server := range servers {
		status, ok := statuses[server.Name]
		if ok {
			out = append(out, mcpServerView(server, &status))
		} else {
			out = append(out, mcpServerView(server, nil))
		}
	}
	return out, nil
}

// MCPServer returns one unified server resource.
func (c *Coordinator) MCPServer(ctx context.Context, name string) (MCPServer, error) {
	if c.mcpRegistry == nil {
		return MCPServer{}, errors.New("integrations: MCP registry is unavailable")
	}
	server, found, err := c.mcpRegistry.Get(ctx, name)
	if err != nil {
		return MCPServer{}, err
	}
	if !found {
		return MCPServer{}, ErrUnknownMCPServer
	}
	status, ok := c.mcpStatusesByName()[name]
	if ok {
		return mcpServerView(server, &status), nil
	}
	return mcpServerView(server, nil), nil
}

func (c *Coordinator) mcpStatusesByName() map[string]MCPServerStatus {
	statuses := make(map[string]MCPServerStatus)
	if c.mcpStatusReader == nil {
		return statuses
	}
	for _, status := range c.mcpStatusReader.Statuses() {
		view := mcpStatusView(status)
		statuses[view.Name] = view
	}
	return statuses
}

// MCPServerStatus resolves one safe live status notification read model.
func (c *Coordinator) MCPServerStatus(_ context.Context, name string) MCPServerStatus {
	if status, ok := c.mcpStatusesByName()[name]; ok {
		return status
	}
	return MCPServerStatus{Name: name}
}
