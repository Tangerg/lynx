package runtime

import (
	"slices"
	"sync"
)

// processRegistry tracks the AgentProcesses the Platform has created.
// Used as a named field on Platform; methods are lowercase since the
// public API lives on Platform (Platform.GetProcess / ActiveProcesses
// forward to get / list here).
//
// Concurrency: a single RWMutex protects the map; registration is
// exclusive, lookups are shared.
type processRegistry struct {
	mu    sync.RWMutex
	procs map[string]*AgentProcess
}

// newProcessRegistry returns an empty registry.
func newProcessRegistry() processRegistry {
	return processRegistry{procs: map[string]*AgentProcess{}}
}

// register adds a process. Called by Platform.createProcess after the
// AgentProcess is fully wired.
func (r *processRegistry) register(p *AgentProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.procs[p.id] = p
}

// get looks up a process by id.
func (r *processRegistry) get(id string) (*AgentProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.procs[id]
	return p, ok
}

// list returns a snapshot of all currently registered processes.
func (r *processRegistry) list() []*AgentProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Collect(func(yield func(*AgentProcess) bool) {
		for _, p := range r.procs {
			if !yield(p) {
				return
			}
		}
	})
}
