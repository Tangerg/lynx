package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

type mcpServerListStub struct {
	servers []mcpserver.Server
	err     error
	calls   int
}

func (s *mcpServerListStub) List(context.Context) ([]mcpserver.Server, error) {
	s.calls++
	return s.servers, s.err
}

func TestBuildMCPEnvironmentUsesOneRegistrySnapshot(t *testing.T) {
	registry := &mcpServerListStub{servers: []mcpserver.Server{
		{Name: "files", Enabled: true, Transport: mcpserver.TransportStdio, Command: "mcp-files", DisabledTools: []string{"write"}, AutoApproveTools: []string{"read"}},
		{Name: "off", Enabled: false, Transport: mcpserver.TransportStdio, Command: "mcp-off", DisabledTools: []string{"hidden"}},
	}}

	env, err := buildMCPEnvironment(context.Background(), registry)
	if err != nil {
		t.Fatalf("buildMCPEnvironment: %v", err)
	}
	if registry.calls != 1 {
		t.Fatalf("registry List calls = %d, want 1", registry.calls)
	}
	if len(env.servers) != 1 || env.servers[0].Name != "files" {
		t.Fatalf("servers = %+v, want enabled files server", env.servers)
	}
	if !env.policy.ToolDisabled(mcpserver.ToolRef{Server: "files", Tool: "write"}) ||
		env.policy.ToolDisabled(mcpserver.ToolRef{Server: "off", Tool: "hidden"}) {
		t.Fatalf("disabled policy does not match registry snapshot")
	}
	if !env.policy.ToolAutoApproved(mcpserver.ToolRef{Server: "files", Tool: "read"}) {
		t.Fatal("files_read must be auto-approved")
	}
}

func TestBuildMCPEnvironmentReturnsRegistryError(t *testing.T) {
	want := errors.New("registry unavailable")
	registry := &mcpServerListStub{err: want}

	_, err := buildMCPEnvironment(context.Background(), registry)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
	if registry.calls != 1 {
		t.Fatalf("registry List calls = %d, want 1", registry.calls)
	}
}
