package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerMCP(registry *Registry) {
	Query(registry, MethodMeta{
		Name:            "mcp.servers.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return router.api.ListMCPServers(ctx)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.servers.create",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
		return router.api.CreateMCPServer(ctx, request)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.servers.update",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
		return router.api.UpdateMCPServer(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "mcp.servers.delete",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPServerRequest) error {
		return router.api.DeleteMCPServer(ctx, request.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	Query(registry, MethodMeta{
		Name:            "mcp.servers.test",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
		return router.api.TestMCPServer(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "mcp.tools.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
		return router.api.ListMCPTools(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "mcp.servers.reconnect",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPServerRequest) error {
		return router.api.ReconnectMCPServer(ctx, request.Server)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.authorizationAttempts.create",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return router.api.CreateMCPAuthorizationAttempt(ctx, request.Server)
	})

	Query(registry, MethodMeta{
		Name:            "mcp.authorizationAttempts.get",
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(router *Router, ctx context.Context, request protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return router.api.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
