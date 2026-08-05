package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerMCP(r *Registry) {
	Query(r, MethodMeta{
		Name:            "mcp.servers.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return d.api.ListMCPServers(ctx)
	})

	Command(r, MethodMeta{
		Name:            "mcp.servers.create",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
		return d.api.CreateMCPServer(ctx, in)
	})

	Command(r, MethodMeta{
		Name:            "mcp.servers.update",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
		return d.api.UpdateMCPServer(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "mcp.servers.delete",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.DeleteMCPServer(ctx, in.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	Query(r, MethodMeta{
		Name:            "mcp.servers.test",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
		return d.api.TestMCPServer(ctx, in)
	})

	Query(r, MethodMeta{
		Name:            "mcp.tools.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
		return d.api.ListMCPTools(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name:            "mcp.servers.reconnect",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPServerRequest) error {
		return d.api.ReconnectMCPServer(ctx, in.Server)
	})

	Command(r, MethodMeta{
		Name:            "mcp.authorizationAttempts.create",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return d.api.CreateMCPAuthorizationAttempt(ctx, in.Server)
	})

	Query(r, MethodMeta{
		Name:            "mcp.authorizationAttempts.get",
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(d *Router, ctx context.Context, in protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return d.api.GetMCPAuthorizationAttempt(ctx, in.AttemptID)
	})
}
