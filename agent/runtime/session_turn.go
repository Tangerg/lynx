package runtime

import (
	"context"
	"sync"
)

type localSessionTurnSequencer struct {
	mu    sync.Mutex
	gates map[string]*sessionTurnGate
}

type sessionTurnGate struct {
	waiters []*sessionTurnWaiter
}

type sessionTurnWaiter struct {
	ready chan struct{}
}

func newLocalSessionTurnSequencer() *localSessionTurnSequencer {
	return &localSessionTurnSequencer{gates: make(map[string]*sessionTurnGate)}
}

func (s *localSessionTurnSequencer) acquire(ctx context.Context, sessionID string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	gate, waiter := s.enqueue(sessionID)
	if waiter == nil {
		return s.releaseFunc(sessionID, gate), nil
	}

	select {
	case <-ctx.Done():
		if s.cancelWaiter(sessionID, gate, waiter) {
			return nil, ctx.Err()
		}
		// Ownership was granted concurrently with cancellation. Pass it on
		// before reporting cancellation so the queue cannot stall.
		s.release(sessionID, gate)
		return nil, ctx.Err()
	case <-waiter.ready:
	}

	// Cancellation and ownership can become ready together. Do not let an
	// already-canceled waiter start a turn merely because select chose ready.
	if err := ctx.Err(); err != nil {
		s.release(sessionID, gate)
		return nil, err
	}
	return s.releaseFunc(sessionID, gate), nil
}

// enqueue defines arrival order at the sequencer's mutex boundary. A nil
// waiter means the caller acquired an idle session immediately.
func (s *localSessionTurnSequencer) enqueue(sessionID string) (*sessionTurnGate, *sessionTurnWaiter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	gate := s.gates[sessionID]
	if gate == nil {
		gate = new(sessionTurnGate)
		s.gates[sessionID] = gate
		return gate, nil
	}
	waiter := &sessionTurnWaiter{ready: make(chan struct{}, 1)}
	gate.waiters = append(gate.waiters, waiter)
	return gate, waiter
}

func (s *localSessionTurnSequencer) cancelWaiter(sessionID string, gate *sessionTurnGate, target *sessionTurnWaiter) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gates[sessionID] != gate {
		return false
	}
	for index, waiter := range gate.waiters {
		if waiter == target {
			copy(gate.waiters[index:], gate.waiters[index+1:])
			gate.waiters[len(gate.waiters)-1] = nil
			gate.waiters = gate.waiters[:len(gate.waiters)-1]
			return true
		}
	}
	return false
}

func (s *localSessionTurnSequencer) releaseFunc(sessionID string, gate *sessionTurnGate) func() {
	var once sync.Once
	return func() {
		once.Do(func() { s.release(sessionID, gate) })
	}
}

func (s *localSessionTurnSequencer) release(sessionID string, gate *sessionTurnGate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gates[sessionID] != gate {
		return
	}
	if len(gate.waiters) == 0 {
		delete(s.gates, sessionID)
		return
	}
	next := gate.waiters[0]
	gate.waiters[0] = nil
	gate.waiters = gate.waiters[1:]
	next.ready <- struct{}{}
}
