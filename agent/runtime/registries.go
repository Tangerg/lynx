package runtime

import (
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// agentRegistry holds the agent definitions a Platform has accepted.
// Re-deploying with the same name replaces the previous registration.
//
// Concurrency: a single RWMutex protects the map; deploys / undeploys
// are exclusive, lookups are shared. Used as a named field on Platform;
// methods are lowercase since the public API lives on Platform itself
// (Platform.Agents / Platform.FindAgent forward to list / find here).
type agentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*core.Agent
}

// newAgentRegistry returns an empty registry.
func newAgentRegistry() agentRegistry {
	return agentRegistry{agents: map[string]*core.Agent{}}
}

// register stores agent under its Name. Must be called only after
// validation succeeds (agentRegistry doesn't validate).
func (r *agentRegistry) register(a *core.Agent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[a.Name] = a
}

// unregister removes the agent at name. Returns an error when the name
// is unknown so callers don't silently miss typos.
func (r *agentRegistry) unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.agents[name]; !ok {
		return fmt.Errorf("agent %q is not deployed", name)
	}
	delete(r.agents, name)
	return nil
}

// list returns a snapshot of registered agents.
func (r *agentRegistry) list() []*core.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*core.Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

// find does a name lookup.
func (r *agentRegistry) find(name string) (*core.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.agents[name]
	return a, ok
}

// processRegistry tracks the AgentProcesses the Platform has created.
// Used as a named field on Platform; methods are lowercase since the
// public API lives on Platform (Platform.ProcessByID / ActiveProcesses
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

// unregister removes the process at id. Returns false when the id is
// unknown so Platform.RemoveProcess can surface a clear error.
func (r *processRegistry) unregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.procs[id]; !ok {
		return false
	}
	delete(r.procs, id)
	return true
}

// pruneWhere deletes every process matching predicate and returns the
// removed ids. Holds the write lock across the sweep so a concurrent
// register doesn't race; predicate must be cheap and side-effect-free.
func (r *processRegistry) pruneWhere(predicate func(*AgentProcess) bool) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var removed []string
	for id, p := range r.procs {
		if predicate(p) {
			delete(r.procs, id)
			removed = append(removed, id)
		}
	}
	return removed
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
