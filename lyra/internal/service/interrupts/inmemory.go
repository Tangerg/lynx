package interrupts

import (
	"context"
	"sync"
)

// inMemory is the default [Store] — a mutex-guarded map keyed by
// ParentRunID. Open interrupts live only as long as the process that
// parked them, which matches same-process resume: the live agent
// process is retained alongside the entry, so both vanish together on
// restart (no dangling, resume-impossible records).
type inMemory struct {
	mu      sync.Mutex
	pending map[string]Pending // parentRunID → entry
}

// NewInMemory returns the in-process [Store].
func NewInMemory() Store {
	return &inMemory{pending: map[string]Pending{}}
}

func (s *inMemory) Put(_ context.Context, p Pending) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[p.ParentRunID] = p
	return nil
}

func (s *inMemory) List(_ context.Context, sessionID string) ([]Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Pending, 0, len(s.pending))
	for _, p := range s.pending {
		if sessionID != "" && p.SessionID != sessionID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *inMemory) Get(_ context.Context, parentRunID string) (Pending, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[parentRunID]
	return p, ok, nil
}

func (s *inMemory) Delete(_ context.Context, parentRunID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, parentRunID)
	return nil
}
