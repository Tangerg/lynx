package turn

import (
	"context"
	"errors"
)

// findTurn looks up the per-turn state by id under the impl's mutex. Returns
// ErrTurnNotFound when the turn has already ended (runTurn deletes itself from
// the map on exit). Centralizes the lock / lookup / unlock sequence every
// public method needs to perform.
func (s *inMemory) findTurn(id string) (*turnState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.turns[id]
	if !ok {
		return nil, ErrTurnNotFound
	}
	return state, nil
}

// SetInterruptKinds records the HITL kinds the connected client can answer
// (from ClientCapabilities.InterruptKinds, negotiated at runtime.initialize).
// Passing an empty slice gates every kind; never calling it leaves the
// permissive default (surface all). Single-tenant: one client's negotiation
// applies process-wide.
func (s *inMemory) SetInterruptKinds(kinds []string) {
	set := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	s.mu.Lock()
	s.interruptKinds = set
	s.mu.Unlock()
}

// canSurface reports whether a turn may park on an interrupt of kind — true
// when no allowlist is configured (permissive default) or kind is in it. A
// false result means the client can't answer this kind, so the turn auto-denies
// instead of deadlocking.
func (s *inMemory) canSurface(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.interruptKinds == nil {
		return true
	}
	return s.interruptKinds[kind]
}

// ProcessID returns the agent-process id backing a live turn — the snapshot key
// the runtime persists so a restart can rebuild the process via [Rehydrate].
// Returns [ErrTurnNotFound] when the turn isn't live.
func (s *inMemory) ProcessID(_ context.Context, handle TurnHandle) (string, error) {
	state, err := s.findTurn(handle.TurnID)
	if err != nil {
		return "", err
	}
	proc := state.process()
	if proc == nil {
		return "", errors.New("turn: turn has not dispatched a process yet")
	}
	return proc.ID(), nil
}
