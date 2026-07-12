// Package mcpstatus carries the capabilities coordinator's MCP connection
// transitions to the single delivery consumer that republishes them on the
// workspace event stream.
package mcpstatus

import (
	"context"
	"sync"
)

// Notifier bridges the capabilities coordinator's MCP reconnect/authorize
// transitions to the one delivery consumer that maps them to wire workspace
// events. Same shape + rationale as filechanges.Notifier (§2.5): the coordinator
// runs the connection fire-and-forget on its own task group and Publishes the
// connecting frame, then the settled frame; delivery Observes and republishes.
//
// Synchronous single-sink by design: Publish invokes the observer inline, so the
// connecting → settled ordering the client relies on is preserved (the two
// Publish calls happen in that order on the coordinator's task goroutine). Lossy
// — a transition with no observer is dropped. The task's context is carried so
// the settled frame's live-status read is scoped to the connection's lifecycle.
type Notifier struct {
	mu   sync.RWMutex
	sink func(ctx context.Context, server string, connecting bool)
}

// Publish forwards one MCP status transition to the installed observer, or drops
// it when none is installed. connecting=true is the transient pre-frame;
// connecting=false is the settled frame. Safe for concurrent use.
func (n *Notifier) Publish(ctx context.Context, server string, connecting bool) {
	n.mu.RLock()
	sink := n.sink
	n.mu.RUnlock()
	if sink != nil {
		sink(ctx, server, connecting)
	}
}

// Observe installs the consumer that receives every subsequent Publish,
// replacing any prior observer.
func (n *Notifier) Observe(sink func(ctx context.Context, server string, connecting bool)) {
	n.mu.Lock()
	n.sink = sink
	n.mu.Unlock()
}
