package goals

import (
	"context"
	"sync"
)

// SessionMutations serializes session lifecycle write-sets with Goal commands
// and owns the in-process registry of active Goal loops. It is created before
// either coordinator, so Session lifecycle coordination never needs a mutable
// Bootstrap proxy to reach a Driver constructed later.
type SessionMutations struct {
	commands sync.Mutex

	mu      sync.Mutex
	running map[string]*loopHandle
}

// NewSessionMutations returns the shared lifecycle coordinator for one runtime.
func NewSessionMutations() *SessionMutations {
	return &SessionMutations{running: map[string]*loopHandle{}}
}

func (m *SessionMutations) lock() { m.commands.Lock() }

func (m *SessionMutations) unlock() { m.commands.Unlock() }

// WithSessionMutation commits apply before quiescing affected Goal loops. A
// failed write leaves the authoritative loop intact.
func (m *SessionMutations) WithSessionMutation(ctx context.Context, sessionIDs []string, apply func(context.Context) error) error {
	m.lock()
	defer m.unlock()
	if err := apply(ctx); err != nil {
		return err
	}
	for _, sessionID := range sessionIDs {
		m.quiesce(sessionID)
	}
	return nil
}

func (m *SessionMutations) launch(sessionID string, handle *loopHandle) {
	m.mu.Lock()
	if m.running == nil {
		m.running = map[string]*loopHandle{}
	}
	if prior := m.running[sessionID]; prior != nil {
		prior.cancel()
	}
	m.running[sessionID] = handle
	m.mu.Unlock()
}

func (m *SessionMutations) forget(sessionID string, handle *loopHandle) {
	m.mu.Lock()
	if m.running[sessionID] == handle {
		delete(m.running, sessionID)
	}
	m.mu.Unlock()
}

func (m *SessionMutations) quiesce(sessionID string) {
	m.mu.Lock()
	if handle := m.running[sessionID]; handle != nil {
		handle.cancel()
		delete(m.running, sessionID)
	}
	m.mu.Unlock()
}
