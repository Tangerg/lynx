package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	MCPServersList                 Name = "mcp.servers.list"
	MCPServersCreate               Name = "mcp.servers.create"
	MCPServersUpdate               Name = "mcp.servers.update"
	MCPServersDelete               Name = "mcp.servers.delete"
	MCPServersTest                 Name = "mcp.servers.test"
	MCPToolsList                   Name = "mcp.tools.list"
	MCPServersReconnect            Name = "mcp.servers.reconnect"
	MCPAuthorizationAttemptsCreate Name = "mcp.authorizationAttempts.create"
	MCPAuthorizationAttemptsGet    Name = "mcp.authorizationAttempts.get"
)

func registerMCP(registry *Registry) {
	registry.Query(MethodMeta{
		Name:            MCPServersList,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ListMCPServers(context.Context) (*protocol.Page[protocol.MCPServer], error)
	}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.MCPServer], error) {
		return service.ListMCPServers(ctx)
	})

	registry.Command(MethodMeta{
		Name:            MCPServersCreate,
		Errors:          []string{protocol.ErrMCPServerAlreadyExists.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		CreateMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPServer, error) {
		return service.CreateMCPServer(ctx, request)
	})

	registry.Command(MethodMeta{
		Name:            MCPServersUpdate,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error)
	}, ctx context.Context, request protocol.UpdateMCPServerRequest) (*protocol.MCPServer, error) {
		return service.UpdateMCPServer(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name:            MCPServersDelete,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		DeleteMCPServer(context.Context, string) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.DeleteMCPServer(ctx, request.Server)
	})

	// A connection probe persists nothing, so a retry is not a replay concern.
	registry.Query(MethodMeta{
		Name:            MCPServersTest,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		TestMCPServer(context.Context, protocol.MCPServerCandidate) (*protocol.MCPTestResult, error)
	}, ctx context.Context, request protocol.MCPServerCandidate) (*protocol.MCPTestResult, error) {
		return service.TestMCPServer(ctx, request)
	})

	registry.Query(MethodMeta{
		Name:            MCPToolsList,
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ListMCPTools(context.Context, protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error)
	}, ctx context.Context, request protocol.MCPListToolsRequest) (*protocol.Page[protocol.MCPTool], error) {
		return service.ListMCPTools(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name:            MCPServersReconnect,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		ReconnectMCPServer(context.Context, string) error
	}, ctx context.Context, request protocol.MCPServerRequest) error {
		return service.ReconnectMCPServer(ctx, request.Server)
	})

	registry.Command(MethodMeta{
		Name:            MCPAuthorizationAttemptsCreate,
		Errors:          []string{protocol.ErrMCPServerNotFound.Error(), protocol.ErrMCPServerDisabled.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		CreateMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.CreateMCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.CreateMCPAuthorizationAttempt(ctx, request.Server)
	})

	registry.Query(MethodMeta{
		Name:            MCPAuthorizationAttemptsGet,
		Errors:          []string{protocol.ErrMCPAuthorizationAttemptNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureMCP),
	}, func(service interface {
		GetMCPAuthorizationAttempt(context.Context, string) (*protocol.MCPAuthorizationAttempt, error)
	}, ctx context.Context, request protocol.MCPAuthorizationAttemptRequest) (*protocol.MCPAuthorizationAttempt, error) {
		return service.GetMCPAuthorizationAttempt(ctx, request.AttemptID)
	})
}
