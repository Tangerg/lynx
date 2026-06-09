package engine

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/a2a"
	"github.com/Tangerg/lynx/core/model/chat"
)

// dialA2AAgents resolves and connects every configured remote A2A agent,
// returning the tools (one per agent) alongside the open clients so the
// caller can close them at shutdown. The A2A analogue of [dialMCPServers]:
// each remote agent becomes one delegation tool the chat loop can call.
//
// Failure semantics match the MCP path: any single agent that can't be
// resolved or dialed fails the whole call (the operator sees it at startup
// rather than discovering a missing capability later), and [a2a.DialAll]
// closes the already-opened clients before returning the error.
func dialA2AAgents(ctx context.Context, agents []a2a.ClientConfig) (tools []chat.Tool, clients []*a2aclient.Client, err error) {
	if len(agents) == 0 {
		return nil, nil, nil
	}

	ctx, span := engineTracer.Start(ctx, "a2a.dial_agents",
		trace.WithAttributes(attribute.Int("a2a.agent.count", len(agents))))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	tools, clients, derr := a2a.DialAll(ctx, agents...)
	if derr != nil {
		return nil, nil, fmt.Errorf("engine: dial A2A agents: %w", derr)
	}

	span.SetAttributes(attribute.Int("a2a.tool.count", len(tools)))
	return tools, clients, nil
}
