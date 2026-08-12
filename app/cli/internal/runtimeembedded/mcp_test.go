package runtimeembedded

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mcp"
)

type mcpBindingStub struct {
	t       *testing.T
	actions []string
	created protocol.MCPServerCandidate
	updated protocol.UpdateMCPServerRequest
	authErr error
	authGet *protocol.MCPAuthorizationAttempt
	now     time.Time
}

func (stub *mcpBindingStub) ListMCPServers(_ context.Context, options embedded.CallOptions) (*protocol.Page[protocol.MCPServer], error) {
	stub.assertMeta(options.RequestMeta)
	return protocol.NewPage([]protocol.MCPServer{wireMCPServer()}), nil
}

func (stub *mcpBindingStub) CreateMCPServer(_ context.Context, request protocol.MCPServerCandidate, options embedded.CommandOptions) (*protocol.MCPServer, error) {
	stub.assertCommand("create", options)
	stub.created = request
	server := wireMCPServer()
	server.Name = request.Name
	return &server, nil
}

func (stub *mcpBindingStub) UpdateMCPServer(_ context.Context, request protocol.UpdateMCPServerRequest, options embedded.CommandOptions) (*protocol.MCPServer, error) {
	stub.assertCommand("update", options)
	stub.updated = request
	server := wireMCPServer()
	server.Name = request.Server
	return &server, nil
}

func (stub *mcpBindingStub) DeleteMCPServer(_ context.Context, request protocol.MCPServerRequest, options embedded.CommandOptions) error {
	stub.assertCommand("delete:"+request.Server, options)
	return nil
}

func (stub *mcpBindingStub) TestMCPServer(_ context.Context, request protocol.MCPServerCandidate, options embedded.CallOptions) (*protocol.MCPTestResult, error) {
	stub.assertMeta(options.RequestMeta)
	stub.actions = append(stub.actions, "test:"+request.Name)
	return &protocol.MCPTestResult{Error: &protocol.ProblemData{Type: protocol.ProblemMCPDialFailed}}, nil
}

func (stub *mcpBindingStub) ListMCPTools(_ context.Context, request protocol.MCPListToolsRequest, options embedded.CallOptions) (*protocol.Page[protocol.MCPTool], error) {
	stub.assertMeta(options.RequestMeta)
	stub.actions = append(stub.actions, "tools:"+request.Server)
	return protocol.NewPage([]protocol.MCPTool{{
		Server: "docs", Name: "search", Description: "Search docs",
		InputSchema: map[string]any{"type": "object"},
	}}), nil
}

func (stub *mcpBindingStub) ReconnectMCPServer(_ context.Context, request protocol.MCPServerRequest, options embedded.CommandOptions) error {
	stub.assertCommand("reconnect:"+request.Server, options)
	return nil
}

func (stub *mcpBindingStub) CreateMCPAuthorizationAttempt(_ context.Context, request protocol.CreateMCPAuthorizationAttemptRequest, options embedded.CommandOptions) (*protocol.MCPAuthorizationAttempt, error) {
	stub.assertCommand("authorize:"+request.Server, options)
	return &protocol.MCPAuthorizationAttempt{
		ID: "auth_1", Server: request.Server, Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptPending},
		CreatedAt: stub.now,
	}, nil
}

func (stub *mcpBindingStub) GetMCPAuthorizationAttempt(_ context.Context, request protocol.MCPAuthorizationAttemptRequest, options embedded.CallOptions) (*protocol.MCPAuthorizationAttempt, error) {
	stub.assertMeta(options.RequestMeta)
	stub.actions = append(stub.actions, "authorization:"+request.AttemptID)
	if stub.authErr != nil {
		return nil, stub.authErr
	}
	if stub.authGet != nil {
		return stub.authGet, nil
	}
	finished := stub.now.Add(time.Second)
	return &protocol.MCPAuthorizationAttempt{
		ID: request.AttemptID, Server: "docs", Status: protocol.MCPAuthorizationAttemptStatus{Type: protocol.MCPAuthorizationAttemptSucceeded},
		CreatedAt: stub.now, FinishedAt: &finished,
	}, nil
}

func (stub *mcpBindingStub) assertMeta(meta protocol.RequestMeta) {
	stub.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		stub.t.Fatalf("MCP request meta = %+v", meta)
	}
}

func (stub *mcpBindingStub) assertCommand(action string, options embedded.CommandOptions) {
	stub.t.Helper()
	stub.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		stub.t.Fatalf("MCP command options = %+v", options)
	}
	stub.actions = append(stub.actions, action)
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
	if _, err := runtime.CreateServer(t.Context(), candidate); err != nil {
		t.Fatal(err)
	}
	if stub.created.Connection.Authorization == nil || stub.created.Connection.Authorization.Value != "Bearer secret" {
		t.Fatalf("created candidate = %+v", stub.created)
	}
	description := "Updated docs"
	enabled := false
	update := mcp.ServerUpdate{Server: "docs", Enabled: &enabled, Description: &description}
	if _, err := runtime.UpdateServer(t.Context(), update); err != nil {
		t.Fatal(err)
	}
	if stub.updated.Enabled == nil || *stub.updated.Enabled || stub.updated.Description == nil || *stub.updated.Description != description {
		t.Fatalf("updated request = %+v", stub.updated)
	}
	if err := runtime.DeleteServer(t.Context(), "docs"); err != nil {
		t.Fatal(err)
	}
	tested, err := runtime.TestServer(t.Context(), candidate)
	if err != nil || tested.OK || tested.Problem == nil || tested.Problem.Type != "mcp_dial_failed" {
		t.Fatalf("TestServer = (%+v, %v)", tested, err)
	}
	tools, err := runtime.Tools(t.Context(), "docs")
	if err != nil || len(tools) != 1 || string(tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("Tools = (%+v, %v)", tools, err)
	}
	if err := runtime.ReconnectServer(t.Context(), "docs"); err != nil {
		t.Fatal(err)
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
