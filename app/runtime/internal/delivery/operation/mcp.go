package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerMCP(registry *Registry) {
	Query(registry, MethodMeta{
		Name:            "mcp.servers.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		ListMCPServers(context.Context) (*protocol.Page[protocol.MCPServer], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return service.ListMCPServers(ctx)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.servers.create",
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		CreateMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
		return service.CreateMCPServer(ctx, request)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.servers.update",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
		return service.UpdateMCPServer(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "mcp.servers.delete",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		DeleteMCPServer(context.Context, string) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.DeleteMCPServer(ctx, request.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	Query(registry, MethodMeta{
		Name:            "mcp.servers.test",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		TestMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPTestResult, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
		return service.TestMCPServer(ctx, request)
	})

	Query(registry, MethodMeta{
		Name:            "mcp.tools.list",
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		ListMCPTools(context.Context, protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error)
	}, ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
		return service.ListMCPTools(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name:            "mcp.servers.reconnect",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		ReconnectMCPServer(context.Context, string) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.ReconnectMCPServer(ctx, request.Server)
	})

	Command(registry, MethodMeta{
		Name:            "mcp.authorizationAttempts.create",
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		CreateMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.CreateMCPAuthorizationAttempt(ctx, request.Server)
	})

	Query(registry, MethodMeta{
		Name:            "mcp.authorizationAttempts.get",
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
		Stability:       stable,
	}, func(service interface {
		GetMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
