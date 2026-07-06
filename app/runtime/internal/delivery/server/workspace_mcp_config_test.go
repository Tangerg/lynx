package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

type mcpConfigRuntime struct {
	stubRuntime
	servers    map[string]mcpserver.Server
	getErr     error
	configured []mcpserver.Server
}

func (r *mcpConfigRuntime) ListMCPRegisteredServers(context.Context) ([]mcpserver.Server, error) {
	out := make([]mcpserver.Server, 0, len(r.servers))
	for _, srv := range r.servers {
		out = append(out, srv)
	}
	return out, nil
}

func (r *mcpConfigRuntime) GetMCPRegisteredServer(_ context.Context, name string) (mcpserver.Server, bool, error) {
	if r.getErr != nil {
		return mcpserver.Server{}, false, r.getErr
	}
	srv, ok := r.servers[name]
	return srv, ok, nil
}

func (r *mcpConfigRuntime) ConfigureMCPServer(_ context.Context, srv mcpserver.Server) error {
	r.configured = append(r.configured, srv)
	return nil
}

func TestWorkspaceMCPConfigurePreservesStoredAuthorization(t *testing.T) {
	rt := &mcpConfigRuntime{servers: map[string]mcpserver.Server{
		"linear": {
			Name:          "linear",
			Transport:     mcpserver.TransportStreamableHTTP,
			URL:           "https://mcp.linear.app/mcp",
			Authorization: "Bearer stored-token",
		},
	}}
	s := newTestServer(rt)

	got, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:      "linear",
		Transport: mcpserver.TransportStreamableHTTP,
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

func TestWorkspaceMCPConfigurePropagatesAuthorizationLookupError(t *testing.T) {
	lookupErr := errors.New("registry unavailable")
	rt := &mcpConfigRuntime{servers: map[string]mcpserver.Server{}, getErr: lookupErr}
	s := newTestServer(rt)

	_, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:      "linear",
		Transport: mcpserver.TransportStreamableHTTP,
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
	rt := &mcpConfigRuntime{}
	s := newTestServer(rt)

	_, err := s.WorkspaceMCPConfigure(context.Background(), protocol.ConfigureMCPServerRequest{
		Name:           "linear",
		Transport:      mcpserver.TransportStreamableHTTP,
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
