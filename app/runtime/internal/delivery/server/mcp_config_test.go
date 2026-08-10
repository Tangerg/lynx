package server

import (
	"context"
	"errors"
	"testing"

	mcpapp "github.com/Tangerg/lynx/app/runtime/internal/application/mcp"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestUpdateMCPServerPreservesStoredHTTPSecretsAtSameOrigin(t *testing.T) {
	registry := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"linear": {
			Name: "linear", Enabled: true, Transport: mcpserver.TransportStreamableHTTP,
			URL: "https://mcp.linear.app/mcp", Authorization: "Bearer stored-token",
			Headers: map[string]string{"X-API-Key": "stored-key"},
		},
	}}
	s := serverWithMCP(mcpapp.Config{Registry: registry})
	connection := protocol.MCPConnectionInput{
		Type: protocol.MCPTransportStreamableHTTP,
		URL:  "https://mcp.linear.app/other-path",
	}
	got, err := s.UpdateMCPServer(context.Background(), protocol.UpdateMCPServerRequest{
		Server: "linear", Connection: &connection,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(registry.saved) != 1 {
		t.Fatalf("saved %d server(s), want 1", len(registry.saved))
	}
	if registry.saved[0].Authorization != "Bearer stored-token" {
		t.Fatalf("Authorization = %q, want stored token", registry.saved[0].Authorization)
	}
	if got.Connection.AuthorizationMasked == "" || got.Connection.AuthorizationMasked == "Bearer stored-token" {
		t.Fatalf("AuthorizationMasked = %q, want masked stored token", got.Connection.AuthorizationMasked)
	}
	if registry.saved[0].Headers["X-API-Key"] != "stored-key" {
		t.Fatalf("Headers = %#v, want stored header", registry.saved[0].Headers)
	}
	if got.Connection.HeadersMasked["X-API-Key"] == "" || got.Connection.HeadersMasked["X-API-Key"] == "stored-key" {
		t.Fatalf("HeadersMasked = %#v, want masked stored header", got.Connection.HeadersMasked)
	}
}

func TestUpdateMCPServerRequiresExplicitAuthorizationDispositionAcrossOrigins(t *testing.T) {
	registry := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"linear": {
			Name: "linear", Enabled: true, Transport: mcpserver.TransportStreamableHTTP,
			URL: "https://mcp.linear.app/mcp", Authorization: "Bearer stored-token",
		},
	}}
	s := serverWithMCP(mcpapp.Config{Registry: registry})
	connection := protocol.MCPConnectionInput{
		Type: protocol.MCPTransportStreamableHTTP,
		URL:  "https://other.example/mcp",
	}
	_, err := s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "linear", Connection: &connection,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("update across origins = %v, want ErrInvalidParams", err)
	}
	if len(registry.saved) != 0 {
		t.Fatalf("saved %d server(s) after ambiguous credential transfer", len(registry.saved))
	}

	connection.Authorization = &protocol.MCPAuthorizationChange{Type: protocol.MCPSecretClear}
	got, err := s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "linear", Connection: &connection,
	})
	if err != nil {
		t.Fatalf("explicit clear: %v", err)
	}
	if registry.saved[0].Authorization != "" || got.Connection.AuthorizationMasked != "" {
		t.Fatalf("authorization survived explicit clear: stored=%q wire=%q",
			registry.saved[0].Authorization, got.Connection.AuthorizationMasked)
	}
}

func TestUpdateMCPServerRequiresExplicitHeadersDispositionAcrossOrigins(t *testing.T) {
	registry := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"cloud": {
			Name: "cloud", Enabled: true, Transport: mcpserver.TransportStreamableHTTP,
			URL: "https://old.example/mcp", Headers: map[string]string{"X-API-Key": "stored-key"},
		},
	}}
	s := serverWithMCP(mcpapp.Config{Registry: registry})
	connection := protocol.MCPConnectionInput{
		Type: protocol.MCPTransportStreamableHTTP,
		URL:  "https://new.example/mcp",
	}
	_, err := s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "cloud", Connection: &connection,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("update across origins = %v, want ErrInvalidParams", err)
	}

	connection.Headers = &protocol.MCPHeadersChange{Type: protocol.MCPSecretClear}
	got, err := s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "cloud", Connection: &connection,
	})
	if err != nil {
		t.Fatalf("explicit headers clear: %v", err)
	}
	if len(registry.saved[0].Headers) != 0 || len(got.Connection.HeadersMasked) != 0 {
		t.Fatalf("headers survived explicit clear: stored=%#v wire=%#v",
			registry.saved[0].Headers, got.Connection.HeadersMasked)
	}
}

func TestUpdateMCPServerProtectsStoredEnvironmentAcrossProcessTargets(t *testing.T) {
	registry := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"fs": {
			Name: "fs", Enabled: true, Transport: mcpserver.TransportStdio,
			Command: "node", Args: []string{"server.js"}, Dir: "/repo",
			Env: map[string]string{"API_KEY": "stored-key"},
		},
	}}
	s := serverWithMCP(mcpapp.Config{Registry: registry})
	connection := protocol.MCPConnectionInput{
		Type: protocol.MCPTransportStdio, Command: "node", Args: []string{"server.js"}, Dir: "/repo",
	}
	got, err := s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "fs", Connection: &connection,
	})
	if err != nil {
		t.Fatalf("same target update: %v", err)
	}
	if registry.saved[0].Env["API_KEY"] != "stored-key" {
		t.Fatalf("Env = %#v, want stored environment", registry.saved[0].Env)
	}
	if got.Connection.EnvMasked["API_KEY"] == "" || got.Connection.EnvMasked["API_KEY"] == "stored-key" {
		t.Fatalf("EnvMasked = %#v, want masked environment", got.Connection.EnvMasked)
	}

	connection.Args = []string{"other.js"}
	_, err = s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "fs", Connection: &connection,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("changed target update = %v, want ErrInvalidParams", err)
	}

	connection.Env = &protocol.MCPEnvironmentChange{Type: protocol.MCPSecretClear}
	got, err = s.UpdateMCPServer(t.Context(), protocol.UpdateMCPServerRequest{
		Server: "fs", Connection: &connection,
	})
	if err != nil {
		t.Fatalf("explicit environment clear: %v", err)
	}
	if len(registry.saved[len(registry.saved)-1].Env) != 0 || len(got.Connection.EnvMasked) != 0 {
		t.Fatalf("environment survived explicit clear: stored=%#v wire=%#v",
			registry.saved[len(registry.saved)-1].Env, got.Connection.EnvMasked)
	}
}

func TestCreateMCPServerPropagatesExistenceLookupError(t *testing.T) {
	lookupErr := errors.New("registry unavailable")
	registry := &mcpRegistryFake{servers: map[string]mcpserver.Server{}, getErr: lookupErr}
	s := serverWithMCP(mcpapp.Config{Registry: registry})

	_, err := s.CreateMCPServer(context.Background(), protocol.MCPServerCandidate{
		Name: "linear", Enabled: true,
		Connection: protocol.MCPConnectionInput{
			Type: protocol.MCPTransportStreamableHTTP,
			URL:  "https://mcp.linear.app/mcp",
		},
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("create err = %v, want registry lookup error", err)
	}
	if len(registry.saved) != 0 {
		t.Fatalf("saved %d server(s), want none after lookup failure", len(registry.saved))
	}
}

func TestCreateMCPServerRejectsNegativeTimeout(t *testing.T) {
	registry := &mcpRegistryFake{}
	s := serverWithMCP(mcpapp.Config{Registry: registry})

	_, err := s.CreateMCPServer(context.Background(), protocol.MCPServerCandidate{
		Name: "linear", TimeoutSeconds: -1,
		Connection: protocol.MCPConnectionInput{
			Type: protocol.MCPTransportStreamableHTTP,
			URL:  "https://mcp.linear.app/mcp",
		},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("create err = %v, want ErrInvalidParams", err)
	}
	if len(registry.saved) != 0 {
		t.Fatalf("saved %d server(s), want none", len(registry.saved))
	}
}

func TestCreateMCPServerRejectsInvalidHTTPEndpointBeforePersistence(t *testing.T) {
	registry := &mcpRegistryFake{}
	s := serverWithMCP(mcpapp.Config{Registry: registry})

	_, err := s.CreateMCPServer(t.Context(), protocol.MCPServerCandidate{
		Name: "linear",
		Connection: protocol.MCPConnectionInput{
			Type: protocol.MCPTransportStreamableHTTP,
			URL:  "file:///tmp/mcp.sock",
		},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("create err = %v, want ErrInvalidParams", err)
	}
	if len(registry.saved) != 0 {
		t.Fatalf("saved %d server(s), want none", len(registry.saved))
	}
}
