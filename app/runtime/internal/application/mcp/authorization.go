package mcp

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

const (
	authorizationAttemptIDPrefix  = "mcpauth_"
	authorizationAttemptRetention = 10 * time.Minute
)

// AuthorizationAttemptStatus is the lifecycle of one interactive MCP OAuth
// flow. A terminal attempt remains readable for the retention published by
// [Coordinator.AuthorizationAttemptRetention] so clients can recover after a
// transient transport interruption.
type AuthorizationAttemptStatus uint8

const (
	AuthorizationAttemptPending AuthorizationAttemptStatus = iota + 1
	AuthorizationAttemptSucceeded
	AuthorizationAttemptFailed
	AuthorizationAttemptCanceled
)

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
	}, func(outcome connectionOutcome) {
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

func (s *authorizationAttemptStore) create(server string) AuthorizationAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	attempt := AuthorizationAttempt{
		ID: s.newID(), Server: server,
		Status: AuthorizationAttemptPending, CreatedAt: s.now().UTC(),
	}
	if _, exists := s.attempts[attempt.ID]; exists {
		panic("mcp: duplicate MCP authorization attempt id")
	}
	s.attempts[attempt.ID] = attempt
	return attempt
}

func (s *authorizationAttemptStore) get(id string) (AuthorizationAttempt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	attempt, ok := s.attempts[id]
	return cloneAuthorizationAttempt(attempt), ok
}

func (s *authorizationAttemptStore) settle(id string, status AuthorizationAttemptStatus) {
	if status == AuthorizationAttemptPending {
		panic("mcp: cannot settle an MCP authorization attempt as pending")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.attempts[id]
	if !ok || attempt.Status != AuthorizationAttemptPending {
		return
	}
	finishedAt := s.now().UTC()
	attempt.Status = status
	attempt.FinishedAt = &finishedAt
	s.attempts[id] = attempt
}

func (s *authorizationAttemptStore) discard(id string) {
	s.mu.Lock()
	delete(s.attempts, id)
	s.mu.Unlock()
}

func (s *authorizationAttemptStore) purgeExpiredLocked() {
	cutoff := s.now().UTC().Add(-s.retention)
	for id, attempt := range s.attempts {
		if attempt.FinishedAt != nil && !attempt.FinishedAt.After(cutoff) {
			delete(s.attempts, id)
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
