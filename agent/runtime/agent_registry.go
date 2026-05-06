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
// are exclusive, lookups are shared. Used as an embedded field on
// Platform — the methods below promote and become Platform.Deploy /
// Undeploy / Agents / FindAgent.
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

// Agents returns a snapshot of registered agents.
func (r *agentRegistry) Agents() []*core.Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*core.Agent, 0, len(r.agents))
	for _, a := range r.agents {
		out = append(out, a)
	}
	return out
}

// FindAgent does a name lookup.
func (r *agentRegistry) FindAgent(name string) (*core.Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.agents[name]
	return a, ok
}
