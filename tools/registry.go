package tools

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

var (
	// ErrDuplicateTool reports an attempt to register a name more than once.
	ErrDuplicateTool = errors.New("tools: duplicate tool")
	// ErrInvalidRegistry reports an operation on a nil Registry receiver.
	ErrInvalidRegistry = errors.New("tools: invalid registry")
)

type entry struct {
	tool       toolcontract.Tool
	definition chat.ToolDefinition
}

// Registry is an instance-scoped, concurrency-safe collection of executable
// tools. Its zero value is ready to use. Each runtime or process owns its
// registry explicitly; there is no package-global counterpart.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]entry
}

// NewRegistry constructs a Registry and atomically registers initial. It
// returns an error without a partially populated registry when any tool is
// invalid or duplicates another name.
func NewRegistry(initial ...toolcontract.Tool) (*Registry, error) {
	registry := &Registry{}
	if err := registry.Register(initial...); err != nil {
		return nil, err
	}
	return registry, nil
}

// Register atomically adds values. A name may be registered only once;
// callers must build a new Registry when they need a different tool set.
func (r *Registry) Register(values ...toolcontract.Tool) error {
	if r == nil {
		return ErrInvalidRegistry
	}
	if len(values) == 0 {
		return nil
	}

	pending := make(map[string]entry, len(values))
	for index, value := range values {
		if nilTool(value) {
			return fmt.Errorf("%w: tools[%d] is nil", toolcontract.ErrInvalidTool, index)
		}
		definition := value.Definition().Clone()
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("%w: tools[%d] definition: %w", toolcontract.ErrInvalidTool, index, err)
		}
		if _, duplicate := pending[definition.Name]; duplicate {
			return fmt.Errorf("%w: %q appears more than once in batch", ErrDuplicateTool, definition.Name)
		}
		pending[definition.Name] = entry{tool: value, definition: definition}
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
	for name, value := range pending {
		r.entries[name] = value
	}
	return nil
}

// Resolve returns the executable tool registered under name.
func (r *Registry) Resolve(name string) (toolcontract.Tool, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.entries[name]
	return value.tool, ok
}

// Definitions returns defensive copies of model-visible definitions sorted by
// name. The stable order makes request construction and tests deterministic.
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

func nilTool(value toolcontract.Tool) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
