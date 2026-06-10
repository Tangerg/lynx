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
// names are silently dropped — first writer wins.
func (r *registry) register(tools ...chat.Tool) {
	if len(tools) == 0 {
		return
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
}

// find looks up a tool by name.
func (r *registry) find(name string) (chat.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	return t, exists
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
