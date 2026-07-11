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
