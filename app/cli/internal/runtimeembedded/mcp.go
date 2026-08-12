package runtimeembedded

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

type mcpBinding interface {
	ListMCPServers(context.Context, embedded.CallOptions) (*protocol.Page[protocol.MCPServer], error)
	CreateMCPServer(context.Context, protocol.MCPServerCandidate, embedded.CommandOptions) (*protocol.MCPServer, error)
	UpdateMCPServer(context.Context, protocol.UpdateMCPServerRequest, embedded.CommandOptions) (*protocol.MCPServer, error)
	DeleteMCPServer(context.Context, protocol.MCPServerRequest, embedded.CommandOptions) error
	TestMCPServer(context.Context, protocol.MCPServerCandidate, embedded.CallOptions) (*protocol.MCPTestResult, error)
	ListMCPTools(context.Context, protocol.MCPListToolsRequest, embedded.CallOptions) (*protocol.Page[protocol.MCPTool], error)
	ReconnectMCPServer(context.Context, protocol.MCPServerRequest, embedded.CommandOptions) error
	CreateMCPAuthorizationAttempt(context.Context, protocol.CreateMCPAuthorizationAttemptRequest, embedded.CommandOptions) (*protocol.MCPAuthorizationAttempt, error)
	GetMCPAuthorizationAttempt(context.Context, protocol.MCPAuthorizationAttemptRequest, embedded.CallOptions) (*protocol.MCPAuthorizationAttempt, error)
}

var _ mcp.Service = (*Runtime)(nil)

func (r *Runtime) Servers(ctx context.Context) ([]mcp.Server, error) {
	page, err := r.mcp.ListMCPServers(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list MCP servers", page)
	if err != nil {
		return nil, err
	}
	servers := make([]mcp.Server, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		server := projectMCPServer(value)
		if err := server.Validate(); err != nil {
			return nil, fmt.Errorf("list MCP servers item %d: %w", index+1, err)
		}
		if _, duplicate := seen[server.Name]; duplicate {
			return nil, fmt.Errorf("list MCP servers repeats %q", server.Name)
		}
		seen[server.Name] = struct{}{}
		servers = append(servers, server)
	}
	return servers, nil
}

func (r *Runtime) CreateServer(ctx context.Context, candidate mcp.Candidate) (mcp.Server, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.Server{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.Server{}, err
	}
	result, err := r.mcp.CreateMCPServer(ctx, projectMCPCandidate(candidate), options)
	return projectMCPServerResult("create MCP server", result, err)
}

func (r *Runtime) UpdateServer(ctx context.Context, update mcp.ServerUpdate) (mcp.Server, error) {
	if err := update.Validate(); err != nil {
		return mcp.Server{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.Server{}, err
	}
	request := protocol.UpdateMCPServerRequest{
		Server: update.Server, Enabled: clonePointer(update.Enabled), Description: clonePointer(update.Description),
		TimeoutSeconds: clonePointer(update.TimeoutSeconds),
	}
	if update.Connection != nil {
		connection := projectMCPConnectionInput(*update.Connection)
		request.Connection = &connection
	}
	if update.DisabledTools != nil {
		values := slices.Clone(*update.DisabledTools)
		request.DisabledTools = &values
	}
	if update.AutoApproveTools != nil {
		values := slices.Clone(*update.AutoApproveTools)
		request.AutoApproveTools = &values
	}
	result, err := r.mcp.UpdateMCPServer(ctx, request, options)
	return projectMCPServerResult("update MCP server", result, err)
}

func (r *Runtime) DeleteServer(ctx context.Context, server string) error {
	return r.mutateMCPServer(ctx, "delete MCP server", server, r.mcp.DeleteMCPServer)
}

func (r *Runtime) ReconnectServer(ctx context.Context, server string) error {
	return r.mutateMCPServer(ctx, "reconnect MCP server", server, r.mcp.ReconnectMCPServer)
}

func (r *Runtime) mutateMCPServer(
	ctx context.Context,
	operation, server string,
	mutate func(context.Context, protocol.MCPServerRequest, embedded.CommandOptions) error,
) error {
	server = strings.TrimSpace(server)
	if server == "" {
		return fmt.Errorf("%s: server name is empty", operation)
	}
	options, err := r.commandOptions()
	if err != nil {
		return err
	}
	return classifyError(mutate(ctx, protocol.MCPServerRequest{Server: server}, options))
}

func (r *Runtime) TestServer(ctx context.Context, candidate mcp.Candidate) (mcp.TestResult, error) {
	if err := candidate.Validate(); err != nil {
		return mcp.TestResult{}, err
	}
	result, err := r.mcp.TestMCPServer(ctx, projectMCPCandidate(candidate), r.callOptions())
	if err != nil {
		return mcp.TestResult{}, classifyError(err)
	}
	if result == nil {
		return mcp.TestResult{}, errors.New("test MCP server: runtime returned nil")
	}
	projected := mcp.TestResult{OK: result.OK, Problem: projectRuntimeProblem(result.Error)}
	if err := projected.Validate(); err != nil {
		return mcp.TestResult{}, fmt.Errorf("test MCP server: %w", err)
	}
	return projected, nil
}

func (r *Runtime) Tools(ctx context.Context, server string) ([]mcp.Tool, error) {
	server = strings.TrimSpace(server)
	page, err := r.mcp.ListMCPTools(ctx, protocol.MCPListToolsRequest{Server: server}, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list MCP tools", page)
	if err != nil {
		return nil, err
	}
	tools := make([]mcp.Tool, 0, len(values))
	seen := make(map[[2]string]struct{}, len(values))
	for index, value := range values {
		tool := mcp.Tool{Server: value.Server, Name: value.Name, Description: value.Description}
		if value.InputSchema != nil {
			schema, marshalErr := json.Marshal(value.InputSchema)
			if marshalErr != nil {
				return nil, fmt.Errorf("list MCP tools item %d schema: %w", index+1, marshalErr)
			}
			tool.InputSchema = schema
		}
		if err := tool.Validate(); err != nil {
			return nil, fmt.Errorf("list MCP tools item %d: %w", index+1, err)
		}
		if server != "" && tool.Server != server {
			return nil, fmt.Errorf("list MCP tools for %q returned a tool from %q", server, tool.Server)
		}
		identity := [2]string{tool.Server, tool.Name}
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf("list MCP tools repeats %s/%s", tool.Server, tool.Name)
		}
		seen[identity] = struct{}{}
		tools = append(tools, tool)
	}
	return tools, nil
}

func (r *Runtime) StartAuthorization(ctx context.Context, server string) (mcp.AuthorizationAttempt, error) {
	server = strings.TrimSpace(server)
	if server == "" {
		return mcp.AuthorizationAttempt{}, errors.New("start MCP authorization: server name is empty")
	}
	options, err := r.commandOptions()
	if err != nil {
		return mcp.AuthorizationAttempt{}, err
	}
	result, err := r.mcp.CreateMCPAuthorizationAttempt(ctx, protocol.CreateMCPAuthorizationAttemptRequest{Server: server}, options)
	return projectMCPAuthorizationResult("start MCP authorization", result, err)
}

func (r *Runtime) GetAuthorization(ctx context.Context, attemptID string) (mcp.AuthorizationAttempt, error) {
	attemptID = strings.TrimSpace(attemptID)
	if attemptID == "" {
		return mcp.AuthorizationAttempt{}, errors.New("get MCP authorization: attempt id is empty")
	}
	result, err := r.mcp.GetMCPAuthorizationAttempt(ctx, protocol.MCPAuthorizationAttemptRequest{AttemptID: attemptID}, r.callOptions())
	return projectMCPAuthorizationResult("get MCP authorization", result, err)
}

func projectMCPServerResult(operation string, result *protocol.MCPServer, err error) (mcp.Server, error) {
	if err != nil {
		return mcp.Server{}, classifyError(err)
	}
	if result == nil {
		return mcp.Server{}, fmt.Errorf("%s: runtime returned nil", operation)
	}
	server := projectMCPServer(*result)
	if err := server.Validate(); err != nil {
		return mcp.Server{}, fmt.Errorf("%s: %w", operation, err)
	}
	return server, nil
}

func projectMCPServer(value protocol.MCPServer) mcp.Server {
	return mcp.Server{
		Name: value.Name, Description: value.Description,
		Connection: mcp.Connection{
			Transport: mcp.Transport(value.Connection.Type), URL: value.Connection.URL,
			AuthorizationMasked: value.Connection.AuthorizationMasked,
			HeadersMasked:       maps.Clone(value.Connection.HeadersMasked),
			Command:             value.Connection.Command, Args: slices.Clone(value.Connection.Args),
			EnvironmentMasked: maps.Clone(value.Connection.EnvMasked), Directory: value.Connection.Dir,
		},
		TimeoutSeconds: value.TimeoutSeconds, DisabledTools: slices.Clone(value.DisabledTools),
		AutoApproveTools: slices.Clone(value.AutoApproveTools),
		State: mcp.State{
			Type: mcp.StateType(value.Status.Type), ToolCount: clonePointer(value.Status.ToolCount),
			Problem: projectRuntimeProblem(value.Status.Error),
		},
	}
}

func projectMCPCandidate(candidate mcp.Candidate) protocol.MCPServerCandidate {
	return protocol.MCPServerCandidate{
		Name: candidate.Name, Enabled: candidate.Enabled, Description: candidate.Description,
		Connection: projectMCPConnectionInput(candidate.Connection), TimeoutSeconds: candidate.TimeoutSeconds,
		DisabledTools: slices.Clone(candidate.DisabledTools), AutoApproveTools: slices.Clone(candidate.AutoApproveTools),
	}
}

func projectMCPConnectionInput(connection mcp.ConnectionInput) protocol.MCPConnectionInput {
	projected := protocol.MCPConnectionInput{
		Type: protocol.MCPTransport(connection.Transport), URL: connection.URL,
		Command: connection.Command, Args: slices.Clone(connection.Args), Dir: connection.Directory,
	}
	if connection.Authorization != nil {
		projected.Authorization = &protocol.MCPAuthorizationChange{
			Type: protocol.MCPSecretChangeType(connection.Authorization.Kind), Value: connection.Authorization.Value,
		}
	}
	if connection.Headers != nil {
		projected.Headers = &protocol.MCPHeadersChange{
			Type: protocol.MCPSecretChangeType(connection.Headers.Kind), Value: maps.Clone(connection.Headers.Value),
		}
	}
	if connection.Environment != nil {
		projected.Env = &protocol.MCPEnvironmentChange{
			Type: protocol.MCPSecretChangeType(connection.Environment.Kind), Value: maps.Clone(connection.Environment.Value),
		}
	}
	return projected
}

func projectMCPAuthorizationResult(operation string, result *protocol.MCPAuthorizationAttempt, err error) (mcp.AuthorizationAttempt, error) {
	if err != nil {
		return mcp.AuthorizationAttempt{}, classifyError(err)
	}
	if result == nil {
		return mcp.AuthorizationAttempt{}, fmt.Errorf("%s: runtime returned nil", operation)
	}
	attempt := mcp.AuthorizationAttempt{
		ID: result.ID, Server: result.Server, Status: mcp.AuthorizationStatus(result.Status.Type),
		Problem: projectRuntimeProblem(result.Status.Error), CreatedAt: result.CreatedAt,
		FinishedAt: clonePointer(result.FinishedAt),
	}
	if err := attempt.Validate(); err != nil {
		return mcp.AuthorizationAttempt{}, fmt.Errorf("%s: %w", operation, err)
	}
	return attempt, nil
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}
