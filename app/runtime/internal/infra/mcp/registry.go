package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
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
func (c *Connections) Statuses() []mcpserver.ConnectionStatus {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]mcpserver.ConnectionStatus, 0, len(c.servers))
	for _, ms := range c.servers {
		out = append(out, mcpserver.ConnectionStatus{Name: ms.name(), State: ms.state, Err: ms.lastErr})
	}
	return out
}

// Tools lists the tools advertised by the connected servers, scoped to server
// when non-empty. It queries each session's tools/list live, ordered by
// (server, tool name) as dialed. Nil-safe.
func (c *Connections) Tools(ctx context.Context, server string) ([]mcpserver.ToolInfo, error) {
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

	var out []mcpserver.ToolInfo
	for _, t := range targets {
		for descriptor, err := range t.session.Tools(ctx, nil) {
			if err != nil {
				return nil, fmt.Errorf("mcp: list tools from server %q: %w", t.name, err)
			}
			schema, err := inputSchema(descriptor.InputSchema)
			if err != nil {
				return nil, fmt.Errorf(
					"mcp: decode input schema for tool %q from server %q: %w",
					descriptor.Name,
					t.name,
					err,
				)
			}
			out = append(out, mcpserver.ToolInfo{
				Server:      t.name,
				Name:        descriptor.Name,
				Description: descriptor.Description,
				InputSchema: schema,
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
	if index := slices.IndexFunc(c.servers, func(ms *server) bool { return ms.name() == name }); index >= 0 {
		old = c.servers[index].session
		// slices.Delete clears the vacated pointer, so the long-lived backing
		// array cannot retain the removed session and its verified tool wrappers.
		c.servers = slices.Delete(c.servers, index, index+1)
	}
	c.mu.Unlock()

	// Shrink the model-facing catalog before a potentially-blocking session
	// close. The mutation lock keeps this publication ordered with every dial.
	c.publishTools()

	if old != nil {
		recordCleanupError(ctx, old.Close())
	}
}

// publishTools rebuilds the model-facing catalog from each connected server's
// last verified tool snapshot. Network I/O happens only while establishing that
// server's session; publication itself is deterministic and cannot turn caller
// cancellation or another server's transient failure into a false catalog.
func (c *Connections) publishTools() {
	c.mu.Lock()
	var catalog []tools.Tool
	for _, ms := range c.servers {
		if ms.session != nil {
			catalog = append(catalog, ms.tools...)
		}
	}
	sink := c.onTools
	c.mu.Unlock()

	if sink != nil {
		sink(catalog)
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
