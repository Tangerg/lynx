// Package a2a is the A2A (Agent-to-Agent) connection infra: it resolves and
// dials configured remote agents (each becomes one delegation tool the chat
// loop can call) and closes them at shutdown. A thin adapter over the lynx a2a
// module; pure infra, no domain knowledge.
package a2a

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	lynxa2a "github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/core/model/chat"
)

// ClientConfig is the remote-agent descriptor, re-exported so callers
// configure connections without importing the lynx a2a module directly.
type ClientConfig = lynxa2a.Endpoint

var tracer = otel.Tracer("lynx/lyra/infra/a2a")

// Connections holds the open remote-agent clients so the caller can close them
// at shutdown.
type Connections struct {
	clients []*a2aclient.Client
}

// Dial resolves and connects every configured remote A2A agent, returning the
// delegation tools (one per agent) alongside the Connections handle.
//
// Failure semantics match the MCP path: any single agent that can't be
// resolved or dialed fails the whole call (the operator sees it at startup
// rather than discovering a missing capability later), and the lynx a2a module
// closes the already-opened clients before returning the error.
func Dial(ctx context.Context, agents []ClientConfig) (*Connections, []chat.Tool, error) {
	if len(agents) == 0 {
		return &Connections{}, nil, nil
	}

	ctx, span := tracer.Start(ctx, "a2a.dial_agents",
		trace.WithAttributes(attribute.Int("a2a.agent.count", len(agents))))
	var err error
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	tools, clients, derr := lynxa2a.Tools(ctx, agents...)
	if derr != nil {
		err = fmt.Errorf("a2a: dial agents: %w", derr)
		return nil, nil, err
	}

	span.SetAttributes(attribute.Int("a2a.tool.count", len(tools)))
	return &Connections{clients: clients}, tools, nil
}

// Close closes every open remote-agent client. Nil-safe; errors are joined.
func (c *Connections) Close() error {
	if c == nil {
		return nil
	}
	return lynxa2a.CloseClients(c.clients)
}
