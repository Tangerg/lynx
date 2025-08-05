package tool

import (
	stdContext "context"
	"maps"
	"sync"
)

// Context provides thread-safe execution context for tool operations with key-value storage.
// Designed for LLM tool execution scenarios where concurrent access and data isolation are required.
//
// Key features:
// - Thread-safe concurrent access from multiple goroutines
// - Integration with standard Go context for cancellation and timeouts
// - Immutable snapshots through cloning operations
//
// Must be created using NewContext() - direct struct instantiation is not supported.
type Context interface {
	// Context returns the underlying context.Context for standard Go context operations
	// including cancellation, deadlines, and request-scoped values.
	Context() stdContext.Context

	// Set stores a key-value pair in the context storage.
	// Empty keys are ignored. Returns the context for method chaining.
	Set(key string, val any) Context

	// SetMap bulk stores key-value pairs from the provided map.
	// Empty keys are ignored, existing keys are overwritten.
	// Returns the context for method chaining.
	SetMap(m map[string]any) Context

	// Get retrieves a value by key from the context storage.
	// Returns value and existence flag - distinguishes between nil values and missing keys.
	Get(key string) (any, bool)

	// Fields returns a deep copy of all stored key-value pairs.
	// Modifications to the returned map do not affect the original context.
	Fields() map[string]any

	// Clear removes all key-value pairs while preserving the underlying context.Context.
	// Useful for context reuse across different tool executions.
	Clear()

	// Clone creates an independent deep copy of the context.
	// The clone has the same underlying context.Context but independent storage.
	// Changes to either the original or clone do not affect each other.
	Clone() Context
}

// context implements the Context interface with thread-safe key-value storage.
type context struct {
	ctx    stdContext.Context
	mu     sync.RWMutex   // Protects concurrent access to fields
	fields map[string]any // Thread-safe key-value storage
}

// NewContext creates a new thread-safe Context instance with the provided context.Context.
func NewContext(ctx stdContext.Context) Context {
	return &context{
		ctx:    ctx,
		fields: make(map[string]any),
	}
}

func (t *context) Context() stdContext.Context {
	if t.ctx == nil {
		return stdContext.Background()
	}
	return t.ctx
}

func (t *context) Set(key string, val any) Context {
	if key == "" {
		return t
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.fields[key] = val
	return t
}

func (t *context) SetMap(m map[string]any) Context {
	if len(m) == 0 {
		return t
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for k, v := range m {
		if k != "" {
			t.fields[k] = v
		}
	}
	return t
}

func (t *context) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	val, ok := t.fields[key]
	return val, ok
}

func (t *context) Fields() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return maps.Clone(t.fields)
}

func (t *context) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	clear(t.fields)
}

func (t *context) Clone() Context {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &context{
		ctx:    t.ctx,
		fields: maps.Clone(t.fields),
	}
}
