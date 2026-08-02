package integrations

import (
	"context"
	"crypto/rand"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

const (
	mcpAuthorizationAttemptIDPrefix  = "mcpauth_"
	mcpAuthorizationAttemptRetention = 10 * time.Minute
)

// MCPAuthorizationAttemptStatus is the lifecycle of one interactive MCP OAuth
// flow. A terminal attempt remains readable for the retention published by
// [Coordinator.MCPAuthorizationAttemptRetention] so clients can recover after a
// transient transport interruption.
type MCPAuthorizationAttemptStatus uint8

const (
	MCPAuthorizationAttemptPending MCPAuthorizationAttemptStatus = iota + 1
	MCPAuthorizationAttemptSucceeded
	MCPAuthorizationAttemptFailed
	MCPAuthorizationAttemptCanceled
)

// MCPAuthorizationAttempt is the application read model for one interactive
// OAuth flow. Failure details remain in telemetry; callers receive the stable
// status and Delivery projects the public problem type.
type MCPAuthorizationAttempt struct {
	ID         string
	Server     string
	Status     MCPAuthorizationAttemptStatus
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// MCPAuthorizationAttemptRetention is the terminal-result window this
// Coordinator enforces. Delivery publishes this exact value in discovery.
func (c *Coordinator) MCPAuthorizationAttemptRetention() time.Duration {
	return c.mcpAuthorizationAttempts.retention
}

// CreateMCPAuthorizationAttempt validates a configured server and starts one
// component-owned interactive OAuth flow. A newer operation for the same server
// cancels this attempt; operations for other servers remain independent.
func (c *Coordinator) CreateMCPAuthorizationAttempt(ctx context.Context, server string) (MCPAuthorizationAttempt, error) {
	target, err := c.mcpConnectionTarget(ctx, server)
	if err != nil {
		return MCPAuthorizationAttempt{}, err
	}
	if target.Transport != mcpserver.TransportStreamableHTTP {
		return MCPAuthorizationAttempt{}, ErrMCPAuthorizationUnsupported
	}
	attempt := c.mcpAuthorizationAttempts.create(server)
	err = c.dispatchMCPConnection(ctx, server, func(ctx context.Context) error {
		return c.mcpConnectionCommands.Authorize(ctx, server)
	}, func(outcome mcpConnectionOutcome) {
		c.mcpAuthorizationAttempts.settle(attempt.ID, authorizationAttemptStatus(outcome))
	})
	if err != nil {
		c.mcpAuthorizationAttempts.discard(attempt.ID)
		return MCPAuthorizationAttempt{}, err
	}
	return attempt, nil
}

// MCPAuthorizationAttempt returns one live or retained terminal attempt.
func (c *Coordinator) MCPAuthorizationAttempt(_ context.Context, id string) (MCPAuthorizationAttempt, error) {
	attempt, ok := c.mcpAuthorizationAttempts.get(id)
	if !ok {
		return MCPAuthorizationAttempt{}, ErrMCPAuthorizationAttemptNotFound
	}
	return attempt, nil
}

func authorizationAttemptStatus(outcome mcpConnectionOutcome) MCPAuthorizationAttemptStatus {
	switch outcome {
	case mcpConnectionSucceeded:
		return MCPAuthorizationAttemptSucceeded
	case mcpConnectionFailed:
		return MCPAuthorizationAttemptFailed
	case mcpConnectionCanceled:
		return MCPAuthorizationAttemptCanceled
	default:
		panic("integrations: unknown MCP connection outcome")
	}
}

type mcpAuthorizationAttemptStore struct {
	mu        sync.Mutex
	now       func() time.Time
	newID     func() string
	retention time.Duration
	attempts  map[string]MCPAuthorizationAttempt
}

func newMCPAuthorizationAttemptStore() *mcpAuthorizationAttemptStore {
	return newMCPAuthorizationAttemptStoreWith(
		time.Now,
		func() string { return mcpAuthorizationAttemptIDPrefix + rand.Text() },
		mcpAuthorizationAttemptRetention,
	)
}

func newMCPAuthorizationAttemptStoreWith(
	now func() time.Time,
	newID func() string,
	retention time.Duration,
) *mcpAuthorizationAttemptStore {
	if now == nil || newID == nil || retention <= 0 {
		panic("integrations: invalid MCP authorization attempt store configuration")
	}
	return &mcpAuthorizationAttemptStore{
		now: now, newID: newID, retention: retention,
		attempts: make(map[string]MCPAuthorizationAttempt),
	}
}

func (s *mcpAuthorizationAttemptStore) create(server string) MCPAuthorizationAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	attempt := MCPAuthorizationAttempt{
		ID: s.newID(), Server: server,
		Status: MCPAuthorizationAttemptPending, CreatedAt: s.now().UTC(),
	}
	if _, exists := s.attempts[attempt.ID]; exists {
		panic("integrations: duplicate MCP authorization attempt id")
	}
	s.attempts[attempt.ID] = attempt
	return attempt
}

func (s *mcpAuthorizationAttemptStore) get(id string) (MCPAuthorizationAttempt, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	attempt, ok := s.attempts[id]
	return cloneMCPAuthorizationAttempt(attempt), ok
}

func (s *mcpAuthorizationAttemptStore) settle(id string, status MCPAuthorizationAttemptStatus) {
	if status == MCPAuthorizationAttemptPending {
		panic("integrations: cannot settle an MCP authorization attempt as pending")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, ok := s.attempts[id]
	if !ok || attempt.Status != MCPAuthorizationAttemptPending {
		return
	}
	finishedAt := s.now().UTC()
	attempt.Status = status
	attempt.FinishedAt = &finishedAt
	s.attempts[id] = attempt
}

func (s *mcpAuthorizationAttemptStore) discard(id string) {
	s.mu.Lock()
	delete(s.attempts, id)
	s.mu.Unlock()
}

func (s *mcpAuthorizationAttemptStore) purgeExpiredLocked() {
	cutoff := s.now().UTC().Add(-s.retention)
	for id, attempt := range s.attempts {
		if attempt.FinishedAt != nil && !attempt.FinishedAt.After(cutoff) {
			delete(s.attempts, id)
		}
	}
}

func cloneMCPAuthorizationAttempt(attempt MCPAuthorizationAttempt) MCPAuthorizationAttempt {
	if attempt.FinishedAt != nil {
		finishedAt := *attempt.FinishedAt
		attempt.FinishedAt = &finishedAt
	}
	return attempt
}
