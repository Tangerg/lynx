package integrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

func TestMCPAuthorizationAttemptReportsFailureWithoutDiscardingResult(t *testing.T) {
	ports := &fakeMCPPorts{
		statuses:     []mcpserver.ConnectionStatus{{Name: "github"}},
		authorizeErr: errors.New("oauth exchange exposed a secret-bearing response"),
	}
	c := New(configWithMCPPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateMCPAuthorizationAttempt(context.Background(), "github")
	if err != nil {
		t.Fatalf("CreateMCPAuthorizationAttempt: %v", err)
	}
	settled := awaitMCPAuthorizationAttempt(t, c, created.ID)
	if settled.Status != MCPAuthorizationAttemptFailed || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want failed", settled)
	}
}

func TestMCPAuthorizationAttemptIsCanceledWhenSuperseded(t *testing.T) {
	authorizeStarted := make(chan string, 1)
	ports := &fakeMCPPorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: "github", State: mcpserver.ConnectionConnected}},
		authorizeStarted: authorizeStarted,
		releaseAuthorize: make(chan struct{}),
	}
	c := New(configWithMCPPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateMCPAuthorizationAttempt(context.Background(), "github")
	if err != nil {
		t.Fatalf("CreateMCPAuthorizationAttempt: %v", err)
	}
	<-authorizeStarted
	if err := c.ReconnectMCPServer(context.Background(), "github"); err != nil {
		t.Fatalf("ReconnectMCPServer: %v", err)
	}
	settled := awaitMCPAuthorizationAttempt(t, c, created.ID)
	if settled.Status != MCPAuthorizationAttemptCanceled || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want canceled", settled)
	}
}

func TestMCPAuthorizationAttemptStoreRetainsOnlyTerminalResults(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := newMCPAuthorizationAttemptStoreWith(
		func() time.Time { return now },
		func() string { return "mcpauth_test" },
		time.Minute,
	)

	pending := store.create("github")
	now = now.Add(2 * time.Minute)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("pending attempt expired")
	}
	store.settle(pending.ID, MCPAuthorizationAttemptSucceeded)
	now = now.Add(time.Minute - time.Nanosecond)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("terminal attempt expired before retention elapsed")
	}
	now = now.Add(time.Nanosecond)
	if _, ok := store.get(pending.ID); ok {
		t.Fatal("terminal attempt survived its retention window")
	}
}

func TestMCPAuthorizationAttemptRejectsUnknownID(t *testing.T) {
	c := New(Config{})
	if _, err := c.MCPAuthorizationAttempt(context.Background(), "mcpauth_missing"); !errors.Is(err, ErrMCPAuthorizationAttemptNotFound) {
		t.Fatalf("MCPAuthorizationAttempt = %v, want ErrMCPAuthorizationAttemptNotFound", err)
	}
}

func TestMCPAuthorizationAttemptRejectsNonHTTPServerBeforeDispatch(t *testing.T) {
	ports := &fakeMCPPorts{statuses: []mcpserver.ConnectionStatus{{Name: "filesystem"}}}
	registry := &testMCPRegistry{servers: map[string]mcpserver.Server{
		"filesystem": {
			Name: "filesystem", Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-filesystem",
		},
	}}
	c := New(Config{
		MCPRegistry: registry, MCPStatusReader: ports,
		MCPConnectionCommands: ports,
	})

	if _, err := c.CreateMCPAuthorizationAttempt(context.Background(), "filesystem"); !errors.Is(err, ErrMCPAuthorizationUnsupported) {
		t.Fatalf("CreateMCPAuthorizationAttempt = %v, want ErrMCPAuthorizationUnsupported", err)
	}
	if ports.authorizeName != "" {
		t.Fatalf("unsupported server dispatched authorization as %q", ports.authorizeName)
	}
}
