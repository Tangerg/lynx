package mcp

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

const (
	authorizationAttemptIDPrefix  = "mcpauth_"
	authorizationAttemptRetention = 10 * time.Minute
)

// AuthorizationAttemptStatus is the lifecycle of one interactive MCP OAuth
// flow. A terminal attempt remains readable for the retention published by
// [Coordinator.AuthorizationAttemptRetention] so clients can recover after a
// transient transport interruption.
type AuthorizationAttemptStatus string

const (
	AuthorizationAttemptPending   AuthorizationAttemptStatus = "pending"
	AuthorizationAttemptSucceeded AuthorizationAttemptStatus = "succeeded"
	AuthorizationAttemptFailed    AuthorizationAttemptStatus = "failed"
	AuthorizationAttemptCanceled  AuthorizationAttemptStatus = "canceled"
)

// Valid reports whether a belongs to the authorization-attempt lifecycle.
func (a AuthorizationAttemptStatus) Valid() bool {
	return a == AuthorizationAttemptPending || a == AuthorizationAttemptSucceeded ||
		a == AuthorizationAttemptFailed || a == AuthorizationAttemptCanceled
}

// String returns the stable authorization-attempt status name.
func (a AuthorizationAttemptStatus) String() string {
	if !a.Valid() {
		return "unknown"
	}
	return string(a)
}

// AuthorizationAttempt is the application read model for one interactive
// OAuth flow. Failure details remain in telemetry; callers receive only the
// stable status.
type AuthorizationAttempt struct {
	ID         string
	Server     string
	Status     AuthorizationAttemptStatus
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// AuthorizationAttemptRetention is the terminal-result window this
// Coordinator enforces and reports to callers.
func (c *Coordinator) AuthorizationAttemptRetention() time.Duration {
	return c.authorizationAttempts.retention
}

// CreateAuthorizationAttempt validates a configured server and starts one
// component-owned interactive OAuth flow. A newer operation for the same server
// cancels this attempt; operations for other servers remain independent.
func (c *Coordinator) CreateAuthorizationAttempt(ctx context.Context, server string) (AuthorizationAttempt, error) {
	target, err := c.connectionTarget(ctx, server)
	if err != nil {
		return AuthorizationAttempt{}, err
	}
	if target.Transport != mcpserver.TransportStreamableHTTP {
		return AuthorizationAttempt{}, ErrAuthorizationUnsupported
	}
	attempt := c.authorizationAttempts.create(server)
	err = c.dispatchConnection(ctx, server, func(ctx context.Context) error {
		return c.connectionControl.Authorize(ctx, server)
	}, true, nil, func(outcome connectionOutcome) {
		c.authorizationAttempts.settle(attempt.ID, authorizationAttemptStatus(outcome))
	})
	if err != nil {
		c.authorizationAttempts.discard(attempt.ID)
		return AuthorizationAttempt{}, err
	}
	return attempt, nil
}

// AuthorizationAttempt returns one live or retained terminal attempt.
func (c *Coordinator) AuthorizationAttempt(_ context.Context, id string) (AuthorizationAttempt, error) {
	attempt, ok := c.authorizationAttempts.get(id)
	if !ok {
		return AuthorizationAttempt{}, ErrAuthorizationAttemptNotFound
	}
	return attempt, nil
}

func authorizationAttemptStatus(outcome connectionOutcome) AuthorizationAttemptStatus {
	switch outcome {
	case connectionSucceeded:
		return AuthorizationAttemptSucceeded
	case connectionFailed:
		return AuthorizationAttemptFailed
	case connectionCanceled:
		return AuthorizationAttemptCanceled
	default:
		panic("mcp: unknown MCP connection outcome")
	}
}

type authorizationAttemptStore struct {
	mu        sync.Mutex
	now       func() time.Time
	newID     func() string
	retention time.Duration
	attempts  map[string]AuthorizationAttempt
}

func newAuthorizationAttemptStore() *authorizationAttemptStore {
	return newAuthorizationAttemptStoreWith(
		time.Now,
		func() string { return authorizationAttemptIDPrefix + rand.Text() },
		authorizationAttemptRetention,
	)
}

func newAuthorizationAttemptStoreWith(
	now func() time.Time,
	newID func() string,
	retention time.Duration,
) *authorizationAttemptStore {
	if now == nil || newID == nil || retention <= 0 {
		panic("mcp: invalid MCP authorization attempt store configuration")
	}
	return &authorizationAttemptStore{
		now: now, newID: newID, retention: retention,
		attempts: make(map[string]AuthorizationAttempt),
	}
}

func (a *authorizationAttemptStore) create(server string) AuthorizationAttempt {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	attempt := AuthorizationAttempt{
		ID: a.newID(), Server: server,
		Status: AuthorizationAttemptPending, CreatedAt: a.now().UTC(),
	}
	if _, exists := a.attempts[attempt.ID]; exists {
		panic("mcp: duplicate MCP authorization attempt id")
	}
	a.attempts[attempt.ID] = attempt
	return attempt
}

func (a *authorizationAttemptStore) get(id string) (AuthorizationAttempt, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeExpiredLocked()
	attempt, ok := a.attempts[id]
	return cloneAuthorizationAttempt(attempt), ok
}

func (a *authorizationAttemptStore) settle(id string, status AuthorizationAttemptStatus) {
	if !status.Valid() || status == AuthorizationAttemptPending {
		panic("mcp: invalid terminal MCP authorization attempt status")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	attempt, ok := a.attempts[id]
	if !ok || attempt.Status != AuthorizationAttemptPending {
		return
	}
	finishedAt := a.now().UTC()
	attempt.Status = status
	attempt.FinishedAt = &finishedAt
	a.attempts[id] = attempt
}

func (a *authorizationAttemptStore) discard(id string) {
	a.mu.Lock()
	delete(a.attempts, id)
	a.mu.Unlock()
}

func (a *authorizationAttemptStore) purgeExpiredLocked() {
	cutoff := a.now().UTC().Add(-a.retention)
	for id, attempt := range a.attempts {
		if attempt.FinishedAt != nil && !attempt.FinishedAt.After(cutoff) {
			delete(a.attempts, id)
		}
	}
}

func cloneAuthorizationAttempt(attempt AuthorizationAttempt) AuthorizationAttempt {
	if attempt.FinishedAt != nil {
		finishedAt := *attempt.FinishedAt
		attempt.FinishedAt = &finishedAt
	}
	return attempt
}
