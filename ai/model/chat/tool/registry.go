package tool

import (
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// Registry provides thread-safe management of immutable tool instances for LLM applications.
// Uses tool names as unique identifiers and prevents duplicate registrations.
// All operations are concurrent-safe and work with immutable tools that cannot be modified after creation.
type Registry struct {
	mu    sync.RWMutex    // Protects concurrent access to the store
	store map[string]Tool // Maps tool names to immutable Tool instances
}

// NewRegistry creates a new registry with optional initial capacity.
// Negative capacity values default to 0.
func NewRegistry(cap ...int) *Registry {
	c, _ := pkgSlices.First(cap)
	if c < 0 {
		c = 0
	}
	return &Registry{
		store: make(map[string]Tool, c),
	}
}

// Register adds immutable tools to the registry using their names as identifiers.
// Duplicate names are silently ignored to prevent overwriting existing tools.
// Returns the registry for method chaining.
func (r *Registry) Register(tools ...Tool) *Registry {
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
		if _, exists := r.store[name]; !exists {
			r.store[name] = t
		}
	}
	return r
}

// Unregister removes tools by name from the registry.
// Non-existent names are silently ignored.
// Returns the registry for method chaining.
func (r *Registry) Unregister(names ...string) *Registry {
	if len(names) == 0 {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, name := range names {

		delete(r.store, name)

	}
	return r
}

// Find retrieves a tool by name.
// Returns the tool and true if found, nil and false otherwise.
func (r *Registry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.store[name]
	return t, ok
}

// Exists checks if a tool with the specified name is registered.
func (r *Registry) Exists(name string) bool {
	_, ok := r.Find(name)
	return ok
}

// All returns a copy of all registered tools.
// The returned slice can be safely modified without affecting the registry.
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Tool, 0, len(r.store))
	for _, t := range r.store {
		list = append(list, t)
	}
	return list
}

// Names returns a copy of all registered tool names.
// The returned slice can be safely modified without affecting the registry.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]string, 0, len(r.store))
	for name := range r.store {
		list = append(list, name)
	}
	return list
}

// Size returns the total number of registered tools.
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.store)
}

// Clear removes all tools from the registry.
// Returns the registry for method chaining.
func (r *Registry) Clear() *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.store)
	return r
}
