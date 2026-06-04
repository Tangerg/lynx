package provider

import (
	"cmp"
	"slices"
	"sync"
)

// Repo is the in-memory core of the provider registry: a mutex-guarded map
// keyed by provider id, plus the Restore/snapshot hooks a persistent backend
// builds on. It mirrors [session.Repo] — the [Service] implementations
// (in-memory here, file in internal/storage) compose it instead of each
// re-implementing the same map + lock + CRUD.
//
// Methods are ctx-free (pure in-memory); the Service wrappers add the
// context-aware signature + any persistence.
type Repo struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRepo returns an empty registry core.
func NewRepo() *Repo { return &Repo{providers: map[string]Provider{}} }

// List returns every entry sorted by id (a stable order for the wire / CLI).
func (r *Repo) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Provider) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// Get returns the entry for id; ok is false when unknown.
func (r *Repo) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// Set upserts an entry by id.
func (r *Repo) Set(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID] = p
}

// Restore loads entries into the registry (used by persistent backends on
// open). Existing entries with the same id are overwritten.
func (r *Repo) Restore(list []Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range list {
		r.providers[p.ID] = p
	}
}
