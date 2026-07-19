package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/integrations"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestWorkspaceMCPConfigurePreservesStoredAuthorization(t *testing.T) {
	rt := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"linear": {
			Name:          "linear",
			Transport:     mcpserver.TransportStreamableHTTP,
			URL:           "https://mcp.linear.app/mcp",
			Authorization: "Bearer stored-token",
		},
	}}
	s := serverWithMCP(integrations.Config{MCPRegistry: rt})

	got, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:      "linear",
		Transport: string(mcpserver.TransportStreamableHTTP),
		Enabled:   true,
		URL:       "https://mcp.linear.app/mcp",
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(rt.configured) != 1 {
		t.Fatalf("configured %d server(s), want 1", len(rt.configured))
	}
	if rt.configured[0].Authorization != "Bearer stored-token" {
		t.Fatalf("Authorization = %q, want stored token", rt.configured[0].Authorization)
	}
	if got.AuthorizationMasked == "" || got.AuthorizationMasked == "Bearer stored-token" {
		t.Fatalf("AuthorizationMasked = %q, want masked stored token", got.AuthorizationMasked)
	}
}

func TestWorkspaceMCPConfigureDropsStoredAuthorizationAcrossOrigins(t *testing.T) {
	rt := &mcpRegistryFake{servers: map[string]mcpserver.Server{
		"linear": {
			Name:          "linear",
			Transport:     mcpserver.TransportStreamableHTTP,
			URL:           "https://mcp.linear.app/mcp",
			Authorization: "Bearer stored-token",
		},
	}}
	s := serverWithMCP(integrations.Config{MCPRegistry: rt})

	got, err := s.WorkspaceMCPConfigure(t.Context(), protocol.ConfigureMCPServerRequest{
		Name:      "linear",
		Transport: string(mcpserver.TransportStreamableHTTP),
		Enabled:   true,
		URL:       "https://attacker.example/mcp",
	})
	if err != nil {
		t.Fatalf("configure: %v", err)
	}
	if rt.configured[0].Authorization != "" || got.AuthorizationMasked != "" {
		t.Fatalf("cross-origin authorization was retained: stored=%q wire=%q",
			rt.configured[0].Authorization, got.AuthorizationMasked)
	}
}

func TestWorkspaceMCPConfigurePropagatesAuthorizationLookupError(t *testing.T) {
	lookupErr := errors.New("registry unavailable")
	rt := &mcpRegistryFake{servers: map[string]mcpserver.Server{}, getErr: lookupErr}
	s := serverWithMCP(integrations.Config{MCPRegistry: rt})

	_, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:      "linear",
		Transport: string(mcpserver.TransportStreamableHTTP),
		Enabled:   true,
		URL:       "https://mcp.linear.app/mcp",
	})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("configure err = %v, want registry lookup error", err)
	}
	if len(rt.configured) != 0 {
		t.Fatalf("configured %d server(s), want none after lookup failure", len(rt.configured))
	}
}

func TestWorkspaceMCPConfigureRejectsNegativeTimeout(t *testing.T) {
	rt := &mcpRegistryFake{}
	s := serverWithMCP(integrations.Config{MCPRegistry: rt})

	_, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:           "linear",
		Transport:      string(mcpserver.TransportStreamableHTTP),
		URL:            "https://mcp.linear.app/mcp",
		TimeoutSeconds: -1,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("configure err = %v, want ErrInvalidParams", err)
	}
	if len(rt.configured) != 0 {
		t.Fatalf("configured %d server(s), want none", len(rt.configured))
	}
}
