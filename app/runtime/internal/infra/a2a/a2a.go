// Package a2a is the A2A (Agent-to-Agent) connection infra: it resolves and
// dials configured remote agents (each becomes one delegation tool the chat
// loop can call) and closes them at shutdown. A thin adapter over the lynx a2a
// module; pure infra, no domain knowledge.
package a2a

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	lynxa2a "github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/tools"
)

// ClientConfig is the infrastructure adapter's transport-neutral input. Keep
// it local instead of re-exporting lynxa2a.Endpoint so protocol-library types
// do not escape this package's boundary.
type ClientConfig struct {
	Name              string
	CardURL           string
	AllowedRPCOrigins []string
}

var tracer = otel.Tracer("lynx/lyra/infra/a2a")

// Connections owns the remote-agent connection cleanup for shutdown.
type Connections struct {
	close     func() error
	closeOnce sync.Once
	closeErr  error
}

// Dial resolves and connects every configured remote A2A agent, returning the
// delegation tools (one per agent) alongside the Connections handle.
//
// Failure semantics match the MCP path: any single agent that can't be
// resolved or dialed fails the whole call (the operator sees it at startup
// rather than discovering a missing capability later), and the lynx a2a module
// closes the already-opened clients before returning the error.
func Dial(ctx context.Context, agents []ClientConfig) (*Connections, []tools.Tool, error) {
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

	endpoints := make([]lynxa2a.Endpoint, len(agents))
	for i, agent := range agents {
		endpoints[i] = lynxa2a.Endpoint{
			Name:    agent.Name,
			CardURL: agent.CardURL,
		}
		if len(agent.AllowedRPCOrigins) > 0 {
			endpoints[i].Policy = &lynxa2a.EndpointPolicy{
				AllowedRPCOrigins: slices.Clone(agent.AllowedRPCOrigins),
			}
		}
	}
	tools, closeTools, derr := lynxa2a.Tools(ctx, endpoints...)
	if derr != nil {
		err = fmt.Errorf("a2a: dial agents: %w", derr)
		return nil, nil, err
	}

	span.SetAttributes(attribute.Int("a2a.tool.count", len(tools)))
	return &Connections{close: closeTools}, tools, nil
}

// Close closes every open remote-agent client. Nil-safe; errors are joined.
func (c *Connections) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.close != nil {
			c.closeErr = c.close()
		}
	})
	return c.closeErr
}
