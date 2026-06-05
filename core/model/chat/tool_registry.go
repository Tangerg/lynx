package chat

import (
	"sync"

	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
)

// ToolRegistry is a thread-safe map from tool name to [Tool] instance.
// Registration is idempotent (duplicate names are silently ignored), so
// concurrent boot-time setup is safe.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// newToolRegistry builds an empty registry. capacityHint, if positive,
// preallocates the backing map.
func newToolRegistry(capacityHint ...int) *ToolRegistry {
	capacity, _ := pkgSlices.First(capacityHint)
	if capacity < 0 {
		capacity = 0
	}
	return &ToolRegistry{tools: make(map[string]Tool, capacity)}
}

// Register adds tools using their definition Name as the key. Duplicate
// names are silently dropped — first writer wins. Returns the registry
// for chaining.
func (r *ToolRegistry) Register(tools ...Tool) *ToolRegistry {
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

// Unregister removes tools by name. Unknown names are silently ignored.
// Returns the registry for chaining.
func (r *ToolRegistry) Unregister(names ...string) *ToolRegistry {
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

// Find looks up a tool by name.
func (r *ToolRegistry) Find(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	return t, exists
}

// Exists reports whether a tool with the given name is registered.
func (r *ToolRegistry) Exists(name string) bool {
	_, ok := r.Find(name)
	return ok
}

// All returns a snapshot of every registered tool. Mutations to the
// returned slice do not affect the registry.
func (r *ToolRegistry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// Names returns a snapshot of every registered tool name.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

// Size returns the number of registered tools.
func (r *ToolRegistry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Clear removes every registered tool. Returns the registry for chaining.
func (r *ToolRegistry) Clear() *ToolRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.tools)
	return r
}
