package runtime

import (
	"maps"
	"slices"
	"sync"
)

// processRegistry tracks processes created or restored by an Engine.
type processRegistry struct {
	mu    sync.RWMutex
	items map[string]*Process
}

func newProcessRegistry() processRegistry {
	return processRegistry{items: map[string]*Process{}}
}

func (r *processRegistry) replace(process *Process) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[process.id] = process
}

func (r *processRegistry) insert(process *Process) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[process.id]; exists {
		return false
	}
	r.items[process.id] = process
	return true
}

// registerNew refuses to replace a live process with a restored copy.
func (r *processRegistry) registerNew(process *Process) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.items[process.id]; ok && !existing.Status().IsTerminal() {
		return false
	}
	r.items[process.id] = process
	return true
}

func (r *processRegistry) unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return false
	}
	delete(r.items, id)
	return true
}

func (r *processRegistry) pruneWhere(predicate func(*Process) bool) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []string
	for id, process := range r.items {
		if predicate(process) {
			delete(r.items, id)
			removed = append(removed, id)
		}
	}
	return removed
}

func (r *processRegistry) get(id string) (*Process, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	process, ok := r.items[id]
	return process, ok
}

func (r *processRegistry) list() []*Process {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Collect(maps.Values(r.items))
}
