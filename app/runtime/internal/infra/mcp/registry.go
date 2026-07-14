package mcp

import (
	"context"
	"errors"
	"fmt"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	lynxmcp "github.com/Tangerg/lynx/mcp"
	"github.com/Tangerg/lynx/tools"
)

// target is a connected (name, session) pair snapshotted under the lock so the
// live tools/list RPCs can run outside it.
type target struct {
	name    string
	session *sdkmcp.ClientSession
}

// Statuses returns one entry per CONFIGURED server (connected and failed
// alike), in dial order. Nil-safe.
func (c *Connections) Statuses() []ServerStatus {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ServerStatus, 0, len(c.servers))
	for _, ms := range c.servers {
		out = append(out, ServerStatus{Name: ms.name(), Status: ms.status, Err: ms.lastErr})
	}
	return out
}

// Tools lists the tools advertised by the connected servers, scoped to server
// when non-empty. It queries each session's tools/list live, ordered by
// (server, tool name) as dialed. Nil-safe.
func (c *Connections) Tools(ctx context.Context, server string) ([]ToolInfo, error) {
	if c == nil {
		return nil, nil
	}
	// Snapshot the connected (name, session) pairs under the lock, then run the
	// live tools/list RPCs outside it — a slow upstream mustn't block reconnect
	// or status reads. A session closed by a racing reconnect just errors here.
	c.mu.Lock()
	var targets []target
	for _, ms := range c.servers {
		if ms.session == nil || (server != "" && ms.name() != server) {
			continue
		}
		targets = append(targets, target{ms.name(), ms.session})
	}
	c.mu.Unlock()

	var out []ToolInfo
	for _, t := range targets {
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from server %q: %w", t.name, err)
			}
			out = append(out, ToolInfo{
				Server:      t.name,
				Name:        descriptor.Name,
				Description: descriptor.Description,
				InputSchema: schemaToMap(descriptor.InputSchema),
			})
		}
	}
	return out, nil
}

// Remove drops a server from the live set, closing its session, and refreshes
// the model-facing tool set. Removing an unknown name is a no-op (the registry
// is the source of truth; the live set may legitimately lag it). Disabling a
// server routes here too — it stays in the registry but leaves the live set.
func (c *Connections) Remove(ctx context.Context, name string) {
	if c == nil {
		return
	}
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	var old *sdkmcp.ClientSession
	kept := c.servers[:0]
	for _, ms := range c.servers {
		if ms.name() == name {
			old = ms.session
			continue
		}
		kept = append(kept, ms)
	}
	c.servers = kept
	c.mu.Unlock()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}
	c.refreshTools(ctx)
}

// refreshTools rebuilds the model-facing tool set from the currently-connected
// sessions and pushes it to the tool sink. A per-server tools/list error drops
// just that server's tools. Runs the RPCs outside mu.
func (c *Connections) refreshTools(ctx context.Context) {
	c.mu.Lock()
	var targets []target
	for _, ms := range c.servers {
		if ms.session != nil {
			targets = append(targets, target{ms.name(), ms.session})
		}
	}
	sink := c.onTools
	c.mu.Unlock()

	var tools []tools.Tool
	for _, t := range targets {
		srcTools, err := sourceTools(ctx, lynxmcp.ToolSource{Name: t.name, Session: t.session})
		if err != nil {
			continue // already-failed server; skip its tools
		}
		tools = append(tools, srcTools...)
	}
	if sink != nil {
		sink(tools)
	}
}

// Close shuts down every open session. Safe to call multiple times. Nil-safe.
func (c *Connections) Close() error {
	if c == nil {
		return nil
	}
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	sessions := make([]*sdkmcp.ClientSession, 0, len(c.servers))
	for _, ms := range c.servers {
		if ms.session != nil {
			sessions = append(sessions, ms.session)
		}
	}
	c.servers = nil
	c.mu.Unlock()

	var errs []error
	for _, session := range sessions {
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
