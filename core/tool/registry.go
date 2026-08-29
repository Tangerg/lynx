package tool

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/scope/core/chat"
)

var (
	ErrDuplicateTool   = errors.New("tool: duplicate tool")
	ErrInvalidRegistry = errors.New("tool: invalid registry")
)

type entry struct {
	binding    Binding
	definition chat.ToolDefinition
}

// Registry is an instance-scoped, concurrency-safe collection of executable
// tools. Its zero value is ready to use. Each runtime or process owns its
// registry explicitly; there is no package-global counterpart. Registration is
// atomic for the full batch, definitions are snapshotted at registration time,
// and model-visible views are returned as defensive copies in stable name order.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

func NewRegistry(initial ...Tool) (*Registry, error) {
	registry := &Registry{}
	if err := registry.Register(initial...); err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *Registry) Register(values ...Tool) error {
	if r == nil {
		return ErrInvalidRegistry
	}
	if len(values) == 0 {
		return nil
	}

	pending := make(map[string]entry, len(values))
	for index, value := range values {
		binding, err := Bind(value)
		if err != nil {
			return fmt.Errorf("tools[%d]: %w", index, err)
		}
		definition := binding.Definition()
		if _, duplicate := pending[definition.Name]; duplicate {
			return fmt.Errorf("%w: %q appears more than once in batch", ErrDuplicateTool, definition.Name)
		}
		pending[definition.Name] = entry{binding: binding, definition: definition}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for name := range pending {
		if _, duplicate := r.entries[name]; duplicate {
			return fmt.Errorf("%w: %q is already registered", ErrDuplicateTool, name)
		}
	}
	if r.entries == nil {
		r.entries = make(map[string]entry, len(pending))
	}
	maps.Copy(r.entries, pending)
	return nil
}

func (r *Registry) Resolve(name string) (Binding, bool) {
	if r == nil {
		return Binding{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.entries[name]
	return value.binding, ok
}

func (r *Registry) Definitions() []chat.ToolDefinition {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	definitions := make([]chat.ToolDefinition, 0, len(r.entries))
	for _, value := range r.entries {
		definitions = append(definitions, value.definition.Clone())
	}
	r.mu.RUnlock()

	slices.SortFunc(definitions, func(a, b chat.ToolDefinition) int {
		return strings.Compare(a.Name, b.Name)
	})
	return definitions
}
