package mcp

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
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
	statuses := c.liveStatusesByName()
	c.statusMu.Lock()
	defer c.statusMu.Unlock()
	for name, status := range c.statusOverrides {
		if !status.Known {
			if _, staleLiveEntry := statuses[name]; !staleLiveEntry {
				// The live port has caught up with disable/delete. Absence already
				// projects as unknown, so the tombstone has finished its handoff.
				delete(c.statusOverrides, name)
				continue
			}
		}
		statuses[name] = cloneServerStatus(status)
	}
	return statuses
}

// liveStatusesByName reads the status-port projection without the application's
// transition overlay. Connection settlement must use this source: reading the
// public model there would merely observe the synthetic connecting state that
// the same operation published before dialing.
func (c *Coordinator) liveStatusesByName() map[string]ServerStatus {
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

func (c *Coordinator) liveStatus(name string) ServerStatus {
	if status, ok := c.liveStatusesByName()[name]; ok {
		return status
	}
	return ServerStatus{Name: name}
}

// ServerStatus resolves one safe live status notification read model.
func (c *Coordinator) ServerStatus(_ context.Context, name string) ServerStatus {
	if status, ok := c.statusesByName()[name]; ok {
		return status
	}
	return ServerStatus{Name: name}
}

// acceptStatus makes a transition readable before publishing its invalidation.
// The live status port remains the cold-start source; this overlay owns only
// transitions admitted by this Coordinator, including the synthetic connecting
// state that precedes the connection call.
func (c *Coordinator) acceptStatus(status ServerStatus) {
	c.statusMu.Lock()
	if status.Known && status.State != mcpserver.ConnectionConnecting {
		// Terminal connection states already come from the live status port. Drop
		// the temporary connecting overlay instead of copying that terminal fact
		// into a second long-lived source that could hide later passive changes.
		delete(c.statusOverrides, status.Name)
	} else {
		// Unknown is a tombstone that masks a stale live snapshot after
		// disable/delete; connecting is the application-owned pre-dial transition.
		c.statusOverrides[status.Name] = cloneServerStatus(status)
	}
	c.statusMu.Unlock()
	c.invalidations.Notify(invalidation.ForMCP(status.Name))
}

func cloneServerStatus(status ServerStatus) ServerStatus {
	if status.ToolCount != nil {
		count := *status.ToolCount
		status.ToolCount = &count
	}
	return status
}
