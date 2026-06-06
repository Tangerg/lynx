package tool

import (
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// registry is a thread-safe map from tool name to [chat.Tool] instance.
// Registration is idempotent (duplicate names are silently ignored), so
// concurrent boot-time setup is safe.
type registry struct {
	mu    sync.RWMutex
	tools map[string]chat.Tool
}

// newRegistry builds an empty registry. capacityHint, if positive,
// preallocates the backing map.
func newRegistry(capacityHint ...int) *registry {
	capacity, _ := pkgSlices.First(capacityHint)
	if capacity < 0 {
		capacity = 0
	}
	return &registry{tools: make(map[string]chat.Tool, capacity)}
}

// register adds tools using their definition Name as the key. Duplicate
// names are silently dropped — first writer wins. Returns the registry
// for chaining.
func (r *registry) register(tools ...chat.Tool) *registry {
	if len(tools) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Definition().Name
		if _, exists := r.tools[name]; !exists {
			r.tools[name] = t
		}
	}
	return r
}

// unregister removes tools by name. Unknown names are silently ignored.
// Returns the registry for chaining.
func (r *registry) unregister(names ...string) *registry {
	if len(names) == 0 {
		return r
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		delete(r.tools, name)
	}
	return r
}

// find looks up a tool by name.
func (r *registry) find(name string) (chat.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	return t, exists
}

// exists reports whether a tool with the given name is registered.
func (r *registry) exists(name string) bool {
	_, ok := r.find(name)
	return ok
}

// all returns a snapshot of every registered tool. Mutations to the
// returned slice do not affect the registry.
func (r *registry) all() []chat.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]chat.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// names returns a snapshot of every registered tool name.
func (r *registry) names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

// size returns the number of registered tools.
func (r *registry) size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// clear removes every registered tool. Returns the registry for chaining.
func (r *registry) clear() *registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.tools)
	return r
}
