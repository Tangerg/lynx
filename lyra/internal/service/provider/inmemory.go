package provider

import (
	"cmp"
	"context"
	"slices"
	"sync"
)

// inMemory is the default [Service] — a mutex-guarded map keyed by provider
// id. State is lost on restart; the file / sqlite backends persist it.
type inMemory struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewInMemory returns the in-process registry.
func NewInMemory() Service {
	return &inMemory{providers: map[string]Provider{}}
}

var _ Service = (*inMemory)(nil)

func (s *inMemory) List(_ context.Context) ([]Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedProviders(s.providers), nil
}

func (s *inMemory) Get(_ context.Context, id string) (Provider, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[id]
	return p, ok, nil
}

func (s *inMemory) Configure(_ context.Context, p Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.ID] = p
	return nil
}

// sortedProviders is the shared snapshot helper: the registry's entries as a
// slice sorted by id (stable list order for the wire / CLI). Callers hold the
// read lock.
func sortedProviders(m map[string]Provider) []Provider {
	out := make([]Provider, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Provider) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
