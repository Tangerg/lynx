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

// registerNew refuses to replace a process until its terminal run has released
// finalization ownership. Otherwise the displaced run could persist after the
// restored copy and overwrite its durable revision.
func (r *processRegistry) registerNew(process *Process) bool {
	for {
		r.mu.RLock()
		existing, exists := r.items[process.id]
		r.mu.RUnlock()
		if exists && !existing.state.removable() {
			return false
		}

		r.mu.Lock()
		current, currentExists := r.items[process.id]
		if currentExists == exists && (!exists || current == existing) {
			r.items[process.id] = process
			r.mu.Unlock()
			return true
		}
		r.mu.Unlock()
	}
}

func (r *processRegistry) unregister(process *Process) bool {
	if process == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[process.id] != process {
		return false
	}
	delete(r.items, process.id)
	return true
}

func (r *processRegistry) unregisterClaimedLeaf(process *Process) (found, hasChildren bool) {
	if process == nil {
		return false, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items[process.id] != process {
		return false, false
	}
	for _, candidate := range r.items {
		if candidate.parentID == process.id {
			return true, true
		}
	}
	delete(r.items, process.id)
	return true, false
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
