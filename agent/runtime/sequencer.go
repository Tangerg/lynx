package runtime

import (
	"context"
	"sync"
)

// localSequencer grants FIFO, single-owner access per opaque key within one
// Engine. It backs both session-turn ordering and process-tree save ordering;
// the key is a session id or a process-tree root id depending on the caller.
type localSequencer struct {
	mu    sync.Mutex
	gates map[string]*sequenceGate
}

type sequenceGate struct {
	waiters []*sequenceWaiter
}

type sequenceWaiter struct {
	ready chan struct{}
}

func newLocalSequencer() *localSequencer {
	return &localSequencer{gates: make(map[string]*sequenceGate)}
}

func (s *localSequencer) acquire(ctx context.Context, key string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	gate, waiter := s.enqueue(key)
	if waiter == nil {
		return s.releaseFunc(key, gate), nil
	}

	select {
	case <-ctx.Done():
		if s.cancelWaiter(key, gate, waiter) {
			return nil, ctx.Err()
		}
		// Ownership was granted concurrently with cancellation. Pass it on
		// before reporting cancellation so the queue cannot stall.
		s.release(key, gate)
		return nil, ctx.Err()
	case <-waiter.ready:
	}

	// Cancellation and ownership can become ready together. Do not let an
	// already-canceled waiter start work merely because select chose ready.
	if err := ctx.Err(); err != nil {
		s.release(key, gate)
		return nil, err
	}
	return s.releaseFunc(key, gate), nil
}

// enqueue defines arrival order at the sequencer's mutex boundary. A nil
// waiter means the caller acquired an idle key immediately.
func (s *localSequencer) enqueue(key string) (*sequenceGate, *sequenceWaiter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	gate := s.gates[key]
	if gate == nil {
		gate = new(sequenceGate)
		s.gates[key] = gate
		return gate, nil
	}
	waiter := &sequenceWaiter{ready: make(chan struct{}, 1)}
	gate.waiters = append(gate.waiters, waiter)
	return gate, waiter
}

func (s *localSequencer) cancelWaiter(key string, gate *sequenceGate, target *sequenceWaiter) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gates[key] != gate {
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

func (s *localSequencer) releaseFunc(key string, gate *sequenceGate) func() {
	var once sync.Once
	return func() {
		once.Do(func() { s.release(key, gate) })
	}
}

func (s *localSequencer) release(key string, gate *sequenceGate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.gates[key] != gate {
		return
	}
	if len(gate.waiters) == 0 {
		delete(s.gates, key)
		return
	}
	next := gate.waiters[0]
	gate.waiters[0] = nil
	gate.waiters = gate.waiters[1:]
	next.ready <- struct{}{}
}
