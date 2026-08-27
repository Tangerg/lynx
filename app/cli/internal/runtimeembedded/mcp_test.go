package runtimeembedded

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/mcp"
)

type mcpBindingStub struct {
	t            *testing.T
	actions      []string
	created      protocol.MCPServerCandidate
	updated      protocol.UpdateMCPServerRequest
	authErr      error
	authGet      *protocol.MCPAuthorizationAttempt
	now          time.Time
	createResult *protocol.MCPServer
	updateResult *protocol.MCPServer
}

func (m *mcpBindingStub) ListMCPServers(_ context.Context, options embedded.CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	m.assertMeta(options.RequestMeta)
	return protocol.NewPage([]protocol.MCPServer{wireMCPServer()}), nil
}

func (m *mcpBindingStub) CreateMCPServer(_ context.Context, request protocol.MCPServerCandidate, options embedded.CommandOptions) (*protocol.MCPServer, error) {
	m.assertCommand("create", options)
	m.created = request
	if m.createResult != nil {
		return m.createResult, nil
	}
	server := wireMCPServerFromCandidate(request)
	return &server, nil
}

func (m *mcpBindingStub) UpdateMCPServer(_ context.Context, request protocol.UpdateMCPServerRequest, options embedded.CommandOptions) (*protocol.MCPServer, error) {
	m.assertCommand("update", options)
	m.updated = request
	if m.updateResult != nil {
		return m.updateResult, nil
	}
	server := wireMCPServer()
	server.Name = request.Server
	if request.Enabled != nil {
		if *request.Enabled {
			server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
		} else {
			server.Status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
		}
	}
	if request.Description != nil {
		server.Description = *request.Description
	}
	if request.Connection != nil {
		server.Connection = wireMCPConnection(*request.Connection)
	}
	if request.TimeoutSeconds != nil {
		server.TimeoutSeconds = *request.TimeoutSeconds
	}
	if request.DisabledTools != nil {
		server.DisabledTools = append([]string(nil), (*request.DisabledTools)...)
	}
	if request.AutoApproveTools != nil {
		server.AutoApproveTools = append([]string(nil), (*request.AutoApproveTools)...)
	}
	return &server, nil
}

func (m *mcpBindingStub) DeleteMCPServer(_ context.Context, request protocol.MCPServerRequest, options embedded.CommandOptions) error {
	m.assertCommand("delete:"+request.Server, options)
	return nil
}

func (m *mcpBindingStub) TestMCPServer(_ context.Context, request protocol.MCPServerCandidate, options embedded.CallOptions) (*protocol.MCPTestResult, error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "test:"+request.Name)
	return &protocol.MCPTestResult{Error: &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}}, nil
}

func (m *mcpBindingStub) ListMCPTools(_ context.Context, request protocol.MCPListToolsRequest, options embedded.CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "tools:"+request.Server)
	return protocol.NewPage([]protocol.MCPTool{{
		Server: "docs", Name: "search", Description: "Search docs",
		InputSchema: map[string]any{"type": "object"},
	}}), nil
}

func (m *mcpBindingStub) ReconnectMCPServer(_ context.Context, request protocol.MCPServerRequest, options embedded.CommandOptions) error {
	m.assertCommand("reconnect:"+request.Server, options)
	return nil
}

func (m *mcpBindingStub) CreateMCPAuthorizationAttempt(_ context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options embedded.CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	m.assertCommand("authorize:"+request.Server, options)
	return &protocol.MCPAuthorizationAttempt{
		ID: "auth_1", Server: request.Server, Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending},
		CreatedAt: m.now,
	}, nil
}

func (m *mcpBindingStub) GetMCPAuthorizationAttempt(_ context.Context, request protocol.MCPAuthorizationAttemptRequest, options embedded.CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	m.assertMeta(options.RequestMeta)
	m.actions = append(m.actions, "authorization:"+request.AttemptID)
	if m.authErr != nil {
		return nil, m.authErr
	}
	if m.authGet != nil {
		return m.authGet, nil
	}
	finished := m.now.Add(time.Second)
	return &protocol.MCPAuthorizationAttempt{
		ID: request.AttemptID, Server: "docs", Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: m.now, FinishedAt: &finished,
	}, nil
}

func (m *mcpBindingStub) assertMeta(meta protocol.RequestMeta) {
	m.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		m.t.Fatalf("MCP request meta = %+v", meta)
	}
}

func (m *mcpBindingStub) assertCommand(action string, options embedded.CommandOptions) {
	m.t.Helper()
	m.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		m.t.Fatalf("MCP command options = %+v", options)
	}
	m.actions = append(m.actions, action)
}

func wireMCPServer() protocol.MCPServer {
	count := 1
	return protocol.MCPServer{
		Name: "docs", Description: "Documentation", TimeoutSeconds: 15,
		Connection: protocol.MCPConnection{
			Type: protocol.MCPTransportStreamableHTTP, URL: "https://mcp.example/tools",
			AuthorizationMasked: "Bearer ****", HeadersMasked: map[string]string{"X-Key": "****"},
		},
		Status: protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &count},
	}
}

func wireMCPServerFromCandidate(candidate protocol.MCPServerCandidate) protocol.MCPServer {
	state := protocol.MCPServerState{Type: protocol.MCPServerDisconnected}
	if !candidate.Enabled {
		state.Type = protocol.MCPServerDisabled
	}
	return protocol.MCPServer{
		Name: candidate.Name, Description: candidate.Description,
		Connection: wireMCPConnection(candidate.Connection), TimeoutSeconds: candidate.TimeoutSeconds,
		DisabledTools:    append([]string(nil), candidate.DisabledTools...),
		AutoApproveTools: append([]string(nil), candidate.AutoApproveTools...), Status: state,
	}
}

func wireMCPConnection(input protocol.MCPConnectionInput) protocol.MCPConnection {
	connection := protocol.MCPConnection{
		Type: input.Type, URL: input.URL, Command: input.Command,
		Args: append([]string(nil), input.Args...), Dir: input.Dir,
	}
	if input.Authorization != nil && input.Authorization.Type == protocol.MCPSecretSet {
		connection.AuthorizationMasked = "****"
	}
	if input.Headers != nil && input.Headers.Type == protocol.MCPSecretSet {
		connection.HeadersMasked = make(map[string]string, len(input.Headers.Value))
		for key := range input.Headers.Value {
			connection.HeadersMasked[key] = "****"
		}
	}
	if input.Env != nil && input.Env.Type == protocol.MCPSecretSet {
		connection.EnvMasked = make(map[string]string, len(input.Env.Value))
		for key := range input.Env.Value {
			connection.EnvMasked[key] = "****"
		}
	}
	return connection
}

func TestMCPAdapterProjectsEveryServerToolAndAuthorizationOperation(t *testing.T) {
	stub := &mcpBindingStub{t: t, now: time.Unix(100, 0)}
	runtime := &Runtime{mcp: stub, meta: requestMeta("test")}
	servers, err := runtime.Servers(t.Context())
	if err != nil || len(servers) != 1 || servers[0].State.Type != mcp.Connected || servers[0].Connection.AuthorizationMasked == "" {
		t.Fatalf("Servers = (%+v, %v)", servers, err)
	}
	authorization := mcp.AuthorizationChange{Kind: mcp.Set, Value: "Bearer secret"}
	candidate := mcp.Candidate{
		Name: "new-docs", Enabled: true,
		Connection: mcp.ConnectionInput{Transport: mcp.StreamableHTTP, URL: "https://new.example/tools", Authorization: &authorization},
	}
	if _, createServerErr := runtime.CreateServer(t.Context(), candidate); createServerErr != nil {
		t.Fatal(createServerErr)
	}
	if stub.created.Connection.Authorization == nil || stub.created.Connection.Authorization.Value != "Bearer secret" {
		t.Fatalf("created candidate = %+v", stub.created)
	}
	description := "Updated docs"
	enabled := false
	update := mcp.ServerUpdate{Server: "docs", Enabled: &enabled, Description: &description}
	if _, updateServerErr := runtime.UpdateServer(t.Context(), update); updateServerErr != nil {
		t.Fatal(updateServerErr)
	}
	if stub.updated.Enabled == nil || *stub.updated.Enabled || stub.updated.Description == nil || *stub.updated.Description != description {
		t.Fatalf("updated request = %+v", stub.updated)
	}
	if deleteServerErr := runtime.DeleteServer(t.Context(), "docs"); deleteServerErr != nil {
		t.Fatal(deleteServerErr)
	}
	tested, err := runtime.TestServer(t.Context(), candidate)
	if err != nil || tested.OK || tested.Problem == nil || tested.Problem.Type != "mcp_dial_failed" {
		t.Fatalf("TestServer = (%+v, %v)", tested, err)
	}
	tools, err := runtime.Tools(t.Context(), "docs")
	if err != nil || len(tools) != 1 || string(tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("Tools = (%+v, %v)", tools, err)
	}
	if reconnectServerErr := runtime.ReconnectServer(t.Context(), "docs"); reconnectServerErr != nil {
		t.Fatal(reconnectServerErr)
	}
	attempt, err := runtime.StartAuthorization(t.Context(), "docs")
	if err != nil || !attempt.Pending() || attempt.ID != "auth_1" {
		t.Fatalf("StartAuthorization = (%+v, %v)", attempt, err)
	}
	attempt, err = runtime.GetAuthorization(t.Context(), attempt.Reference())
	if err != nil || attempt.Status != mcp.AuthorizationSucceeded || attempt.FinishedAt == nil {
		t.Fatalf("GetAuthorization = (%+v, %v)", attempt, err)
	}
	if len(stub.actions) != 8 {
		t.Fatalf("MCP actions = %v", stub.actions)
	}
}

func TestMCPAuthorizationAdapterClassifiesAbsenceAndEnforcesReferenceIdentity(t *testing.T) {
	server := wireMCPServer()
	if _, err := projectMCPServerResult("update MCP server", "other", &server, nil); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched MCP server identity = %v, want ErrIncompatibleRuntime", err)
	}

	stub := &mcpBindingStub{t: t, now: time.Unix(100, 0), authErr: protocol.ErrMCPAuthorizationAttemptNotFound}
	runtime := &Runtime{mcp: stub, meta: requestMeta("test")}
	reference := mcp.AuthorizationReference{ID: "auth_1", Server: "docs"}
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, mcp.ErrAuthorizationAttemptNotFound) {
		t.Fatalf("missing authorization = %v, want ErrAuthorizationAttemptNotFound", err)
	}

	finished := stub.now.Add(time.Second)
	stub.authErr = nil
	stub.authGet = &protocol.MCPAuthorizationAttempt{
		ID: "auth_other", Server: "docs",
		Status:    protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: stub.now, FinishedAt: &finished,
	}
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched authorization identity = %v, want ErrIncompatibleRuntime", err)
	}
	stub.authGet.ID = reference.ID
	stub.authGet.Server = "other"
	if _, err := runtime.GetAuthorization(t.Context(), reference); !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("mismatched authorization server = %v, want ErrIncompatibleRuntime", err)
	}
}

func TestMCPAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	authorization := mcp.AuthorizationChange{Kind: mcp.Set, Value: "Bearer secret"}
	candidate := mcp.Candidate{
		Name: "docs", Enabled: true, Description: "Documentation",
		Connection: mcp.ConnectionInput{
			Transport: mcp.StreamableHTTP, URL: "https://mcp.example/tools", Authorization: &authorization,
		},
	}
	createResult := wireMCPServerFromCandidate(projectMCPCandidate(candidate))
	createResult.Description = "ignored"
	description := "Updated"
	enabled := false
	update := mcp.ServerUpdate{Server: candidate.Name, Enabled: &enabled, Description: &description}
	updateResult := wireMCPServer()
	updateResult.Status = protocol.MCPServerState{Type: protocol.MCPServerDisabled}
	updateResult.Description = "ignored"
	tests := []struct {
		name   string
		stub   *mcpBindingStub
		invoke func(*Runtime) error
	}{
		{
			name: "create fields",
			stub: &mcpBindingStub{createResult: &createResult},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.CreateServer(t.Context(), candidate)
				return err
			},
		},
		{
			name: "update fields",
			stub: &mcpBindingStub{updateResult: &updateResult},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.UpdateServer(t.Context(), update)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t, test.stub.now = t, time.Unix(100, 0)
			runtime := &Runtime{mcp: test.stub, meta: requestMeta("test")}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}

func TestMCPAdapterClassifiesBoundedContextErrors(t *testing.T) {
	tests := []struct {
		source error
		target error
	}{
		{protocol.ErrMCPServerNotFound, mcp.ErrServerNotFound},
		{protocol.ErrMCPServerAlreadyExists, mcp.ErrServerAlreadyExists},
		{protocol.ErrMCPServerDisabled, mcp.ErrServerDisabled},
		{protocol.ErrMCPAuthorizationAttemptNotFound, mcp.ErrAuthorizationAttemptNotFound},
	}
	for _, test := range tests {
		classified := classifyMCPError(test.source)
		if !errors.Is(classified, test.source) || !errors.Is(classified, test.target) {
			t.Errorf("classifyMCPError(%v) = %v, want source and bounded-context identities", test.source, classified)
		}
	}
}
