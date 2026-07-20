package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// SessionTurnSequencer grants exclusive ownership of one session turn. Calls
// for the same session ID must acquire ownership in arrival order; calls for
// different IDs may proceed concurrently. Acquire must respect ctx while
// waiting. A successful call must return a non-nil, idempotent release
// function.
//
// The runtime installs a process-local implementation by default. This
// contract does not carry a fencing token, so cross-node execution ownership
// and stale-writer rejection remain Host responsibilities.
type SessionTurnSequencer interface {
	Acquire(ctx context.Context, sessionID string) (release func(), err error)
}

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

func (s *localSessionTurnSequencer) Acquire(ctx context.Context, sessionID string) (func(), error) {
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

func acquireSessionTurn(ctx context.Context, sequencer SessionTurnSequencer, sessionID string) (release func(), err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			release = nil
			err = panicerr.New(fmt.Sprintf("session turn sequencer %T Acquire panicked", sequencer), recovered)
		}
	}()
	release, err = sequencer.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, errors.New("session turn sequencer returned a nil release function")
	}
	return release, nil
}

func releaseSessionTurn(release func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New("session turn sequencer release panicked", recovered)
		}
	}()
	release()
	return nil
}
