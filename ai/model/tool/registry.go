package tool

import (
	pkgSlices "github.com/Tangerg/lynx/pkg/slices"
	"sync"
)

// Registry provides a thread-safe registry for managing tool instances.
// It supports concurrent registration, lookup, and management of tools
// with automatic name-based indexing and duplicate prevention.
//
// The Registry uses the tool's Definition().Name() as the unique identifier
// for registration and lookup operations. Duplicate registrations with the
// same name are silently ignored to prevent overwriting existing tools.
//
// All operations are thread-safe and can be safely called from multiple
// goroutines simultaneously.
type Registry struct {
	mu    sync.RWMutex    // Protects the store map for concurrent access
	store map[string]Tool // Internal storage mapping tool names to Tool instances
}

// NewRegistry creates a new Registry instance with optional initial capacity.
// The cap parameter specifies the initial capacity of the internal map.
// If no cap is provided or a negative value is given, the capacity defaults to 0.
//
// Example:
//
//	registry := NewRegistry()           // Default capacity
//	registry := NewRegistry(10)        // Initial capacity of 10
//	registry := NewRegistry(-1)        // Capacity defaults to 0
func NewRegistry(cap ...int) *Registry {
	c := pkgSlices.AtOr(cap, 0, 0)
	if c < 0 {
		c = 0
	}
	return &Registry{
		store: make(map[string]Tool, c),
	}
}

// Register adds one or more tools to the registry using their names as unique identifiers.
// The tool name is obtained from tool.Definition().Name().
// If a tool with the same name already exists, the registration is silently skipped
// to prevent overwriting existing tools.
// Returns the Registry instance for method chaining.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	registry.Register(tool1, tool2, tool3)
//	registry.Register(tool1).Register(tool2) // Method chaining
func (r *Registry) Register(tools ...Tool) *Registry {
	if len(tools) == 0 {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range tools {
		name := t.Definition().Name()
		_, ok := r.store[name]
		if ok {
			continue
		}
		r.store[name] = t
	}
	return r
}

// Unregister removes one or more tools from the registry by their names.
// If a tool with the specified name does not exist, the operation is silently skipped.
// Returns the Registry instance for method chaining.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	registry.Unregister("tool1", "tool2")
//	registry.Unregister("tool1").Unregister("tool2") // Method chaining
func (r *Registry) Unregister(names ...string) *Registry {
	if len(names) == 0 {
		return r
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, name := range names {
		_, ok := r.store[name]
		if !ok {
			continue
		}
		delete(r.store, name)
	}
	return r
}

// Find retrieves a tool by name from the registry.
// Returns the tool and true if found, otherwise returns nil and false.
// Empty names will always return nil and false.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	if tool, ok := registry.Find("calculator"); ok {
//	    // Use the tool
//	}
func (r *Registry) Find(name string) (Tool, bool) {
	if name == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.store[name]
	return t, ok
}

// Exists checks whether a tool with the specified name is registered.
// Returns true if the tool exists, false otherwise.
// Empty names will always return false.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	if registry.Exists("calculator") {
//	    // Tool is available
//	}
func (r *Registry) Exists(name string) bool {
	_, ok := r.Find(name)
	return ok
}

// All returns a slice containing all registered tools.
// The returned slice is a copy, so modifications to it will not affect
// the registry's internal state.
//
// This method is thread-safe and can be called concurrently.
// The order of tools in the returned slice is not guaranteed.
//
// Example:
//
//	tools := registry.All()
//	fmt.Printf("Found %d tools\n", len(tools))
func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Tool, 0, len(r.store))
	for _, t := range r.store {
		list = append(list, t)
	}
	return list
}

// Names returns a slice containing the names of all registered tools.
// The returned slice is a copy, so modifications to it will not affect
// the registry's internal state.
//
// This method is thread-safe and can be called concurrently.
// The order of names in the returned slice is not guaranteed.
//
// Example:
//
//	names := registry.Names()
//	fmt.Printf("Available tools: %v\n", names)
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]string, 0, len(r.store))
	for name := range r.store {
		list = append(list, name)
	}
	return list
}

// Size returns the total number of tools currently registered.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	count := registry.Size()
//	fmt.Printf("Registry contains %d tools\n", count)
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.store)
}

// Clear removes all tools from the registry, resetting it to an empty state.
// Returns the Registry instance for method chaining.
//
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	registry.Clear() // Remove all tools
func (r *Registry) Clear() *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()

	clear(r.store)
	return r
}
