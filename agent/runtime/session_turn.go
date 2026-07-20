package runtime

import (
	"context"
	"sync"
)

// SessionTurnSequencer grants exclusive ownership of one session turn. Calls
// for the same session ID must be ordered; calls for different IDs may proceed
// concurrently. Acquire must respect ctx while waiting. A successful call must
// return a non-nil, idempotent release function.
//
// The runtime installs a process-local implementation by default. Multi-node
// hosts may provide a distributed lease implementation through [Config].
type SessionTurnSequencer interface {
	Acquire(ctx context.Context, sessionID string) (release func(), err error)
}

type localSessionTurnSequencer struct {
	mu    sync.Mutex
	gates map[string]*sessionTurnGate
}

type sessionTurnGate struct {
	token chan struct{}
	refs  int
}

func newLocalSessionTurnSequencer() *localSessionTurnSequencer {
	return &localSessionTurnSequencer{gates: make(map[string]*sessionTurnGate)}
}

func (s *localSessionTurnSequencer) Acquire(ctx context.Context, sessionID string) (func(), error) {
	gate := s.retain(sessionID)
	select {
	case <-ctx.Done():
		s.releaseRef(sessionID, gate)
		return nil, ctx.Err()
	case <-gate.token:
	}

	// Cancellation and token delivery can become ready together. Do not let an
	// already-canceled waiter start a turn merely because select chose token.
	if err := ctx.Err(); err != nil {
		gate.token <- struct{}{}
		s.releaseRef(sessionID, gate)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			gate.token <- struct{}{}
			s.releaseRef(sessionID, gate)
		})
	}, nil
}

func (s *localSessionTurnSequencer) retain(sessionID string) *sessionTurnGate {
	s.mu.Lock()
	defer s.mu.Unlock()

	gate := s.gates[sessionID]
	if gate == nil {
		gate = &sessionTurnGate{token: make(chan struct{}, 1)}
		gate.token <- struct{}{}
		s.gates[sessionID] = gate
	}
	gate.refs++
	return gate
}

func (s *localSessionTurnSequencer) releaseRef(sessionID string, gate *sessionTurnGate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	gate.refs--
	if gate.refs == 0 && s.gates[sessionID] == gate {
		delete(s.gates, sessionID)
	}
}
