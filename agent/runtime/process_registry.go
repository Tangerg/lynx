package runtime

import (
	"slices"
	"sync"
)

// processRegistry tracks the AgentProcesses the Platform has created.
// Used as an embedded field on Platform — methods promote and become
// Platform.GetProcess / ActiveProcesses (plus internal register/remove
// for the runtime).
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

// GetProcess looks up a process by id (used by the HITL resume API and
// by debug tools).
func (r *processRegistry) GetProcess(id string) (*AgentProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.procs[id]
	return p, ok
}

// ActiveProcesses returns a snapshot of all currently registered
// processes.
func (r *processRegistry) ActiveProcesses() []*AgentProcess {
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
