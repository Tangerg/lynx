package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

func TestAuthorizationAttemptReportsFailureWithoutDiscardingResult(t *testing.T) {
	ports := &fakePorts{
		statuses:     []mcpserver.ConnectionStatus{{Name: "github"}},
		authorizeErr: errors.New("oauth exchange exposed a secret-bearing response"),
	}
	c := New(configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateAuthorizationAttempt(context.Background(), "github")
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptFailed || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want failed", settled)
	}
}

func TestAuthorizationAttemptIsCanceledWhenSuperseded(t *testing.T) {
	authorizeStarted := make(chan string, 1)
	ports := &fakePorts{
		statuses:         []mcpserver.ConnectionStatus{{Name: "github", State: mcpserver.ConnectionConnected}},
		authorizeStarted: authorizeStarted,
		releaseAuthorize: make(chan struct{}),
	}
	c := New(configWithPorts(ports))
	defer requireCoordinatorShutdown(t, c)

	created, err := c.CreateAuthorizationAttempt(context.Background(), "github")
	if err != nil {
		t.Fatalf("CreateAuthorizationAttempt: %v", err)
	}
	<-authorizeStarted
	if err := c.ReconnectServer(context.Background(), "github"); err != nil {
		t.Fatalf("ReconnectServer: %v", err)
	}
	settled := awaitAuthorizationAttempt(t, c, created.ID)
	if settled.Status != AuthorizationAttemptCanceled || settled.FinishedAt == nil {
		t.Fatalf("settled attempt = %+v, want canceled", settled)
	}
}

func TestAuthorizationAttemptStoreRetainsOnlyTerminalResults(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	store := newAuthorizationAttemptStoreWith(
		func() time.Time { return now },
		func() string { return "mcpauth_test" },
		time.Minute,
	)

	pending := store.create("github")
	now = now.Add(2 * time.Minute)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("pending attempt expired")
	}
	store.settle(pending.ID, AuthorizationAttemptSucceeded)
	now = now.Add(time.Minute - time.Nanosecond)
	if _, ok := store.get(pending.ID); !ok {
		t.Fatal("terminal attempt expired before retention elapsed")
	}
	now = now.Add(time.Nanosecond)
	if _, ok := store.get(pending.ID); ok {
		t.Fatal("terminal attempt survived its retention window")
	}
}

func TestAuthorizationAttemptRejectsUnknownID(t *testing.T) {
	c := New(Config{})
	if _, err := c.AuthorizationAttempt(context.Background(), "mcpauth_missing"); !errors.Is(err, ErrAuthorizationAttemptNotFound) {
		t.Fatalf("AuthorizationAttempt = %v, want ErrAuthorizationAttemptNotFound", err)
	}
}

func TestAuthorizationAttemptRejectsNonHTTPServerBeforeDispatch(t *testing.T) {
	ports := &fakePorts{statuses: []mcpserver.ConnectionStatus{{Name: "filesystem"}}}
	registry := &testRegistry{servers: map[string]mcpserver.Server{
		"filesystem": {
			Name: "filesystem", Enabled: true,
			Transport: mcpserver.TransportStdio, Command: "mcp-filesystem",
		},
	}}
	c := New(Config{
		Registry: registry, StatusReader: ports,
		ConnectionControl: ports,
	})

	if _, err := c.CreateAuthorizationAttempt(context.Background(), "filesystem"); !errors.Is(err, ErrAuthorizationUnsupported) {
		t.Fatalf("CreateAuthorizationAttempt = %v, want ErrAuthorizationUnsupported", err)
	}
	if ports.authorizeName != "" {
		t.Fatalf("unsupported server dispatched authorization as %q", ports.authorizeName)
	}
}
