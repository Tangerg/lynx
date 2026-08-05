package mcp

import (
	"context"
	"errors"
)

// Servers returns every durable MCP server enriched with the current live
// status snapshot. The registry determines membership; the live pool is only a
// projection and therefore cannot make a configured or disabled server vanish.
func (c *Coordinator) Servers(ctx context.Context) ([]Server, error) {
	if c.registry == nil {
		return nil, errors.New("mcp: MCP registry is unavailable")
	}
	servers, err := c.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	statuses := c.statusesByName()
	out := make([]Server, 0, len(servers))
	for _, server := range servers {
		status, ok := statuses[server.Name]
		if ok {
			out = append(out, serverView(server, &status))
		} else {
			out = append(out, serverView(server, nil))
		}
	}
	return out, nil
}

// Server returns one unified server resource.
func (c *Coordinator) Server(ctx context.Context, name string) (Server, error) {
	if c.registry == nil {
		return Server{}, errors.New("mcp: MCP registry is unavailable")
	}
	server, found, err := c.registry.Get(ctx, name)
	if err != nil {
		return Server{}, err
	}
	if !found {
		return Server{}, ErrUnknownServer
	}
	status, ok := c.statusesByName()[name]
	if ok {
		return serverView(server, &status), nil
	}
	return serverView(server, nil), nil
}

func (c *Coordinator) statusesByName() map[string]ServerStatus {
	statuses := make(map[string]ServerStatus)
	if c.statusReader == nil {
		return statuses
	}
	for _, status := range c.statusReader.Statuses() {
		view := statusView(status)
		statuses[view.Name] = view
	}
	return statuses
}

// ServerStatus resolves one safe live status notification read model.
func (c *Coordinator) ServerStatus(_ context.Context, name string) ServerStatus {
	if status, ok := c.statusesByName()[name]; ok {
		return status
	}
	return ServerStatus{Name: name}
}
