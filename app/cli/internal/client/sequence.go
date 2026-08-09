package client

import (
	"errors"
	"fmt"
)

// EventSequence is a transactional cursor/identity guard for one contiguous
// subscription window. The callback is applied before the cursor commits, so a
// domain-folding error cannot leave replay state ahead of product state.
type EventSequence struct {
	cursor Cursor
	ids    map[Cursor]string
}

func NewEventSequence(after Cursor) *EventSequence {
	return &EventSequence{cursor: after, ids: make(map[Cursor]string)}
}

func (s *EventSequence) Cursor() Cursor {
	if s == nil {
		return 0
	}
	return s.cursor
}

// Accept validates envelope, ignores a known identical replay, and invokes apply
// exactly once for the next event. A nil callback is a pure sequence guard.
func (s *EventSequence) Accept(envelope Envelope, apply func() error) (ApplyResult, error) {
	if s == nil {
		return ApplyResult{}, errors.New("event sequence is nil")
	}
	if envelope.Cursor == 0 {
		return ApplyResult{}, errors.New("event cursor is zero")
	}
	if envelope.ID == "" {
		return ApplyResult{}, errors.New("event id is empty")
	}
	if s.ids == nil {
		s.ids = make(map[Cursor]string)
	}
	if known, ok := s.ids[envelope.Cursor]; ok {
		if known != envelope.ID {
			return ApplyResult{}, fmt.Errorf("%w at cursor %d: have %s, received %s", ErrEventConflict, envelope.Cursor, known, envelope.ID)
		}
		return ApplyResult{}, nil
	}
	if envelope.Cursor <= s.cursor {
		return ApplyResult{}, fmt.Errorf("%w at cursor %d: cursor predates guarded window ending at %d", ErrEventConflict, envelope.Cursor, s.cursor)
	}
	want := s.cursor + 1
	if envelope.Cursor != want {
		return ApplyResult{}, fmt.Errorf("%w: expected cursor %d, received %d", ErrEventGap, want, envelope.Cursor)
	}
	if apply != nil {
		if err := apply(); err != nil {
			return ApplyResult{}, err
		}
	}
	s.ids[envelope.Cursor] = envelope.ID
	s.cursor = envelope.Cursor
	return ApplyResult{Applied: true}, nil
}
