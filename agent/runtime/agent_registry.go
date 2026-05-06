package runtime

import (
	"fmt"
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
