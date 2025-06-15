package tool

import (
	stdContext "context"
	"maps"
	"sync"
)

// Context represents an execution context for tool operations, providing
// access to contextual data and key-value storage with thread-safe operations.
//
// Context implementations are thread-safe and can be safely accessed from multiple
// goroutines simultaneously. This allows for concurrent tool execution scenarios
// while maintaining data consistency.
//
// Context instances MUST be created using NewContext() constructor and should not be instantiated
// directly with struct literals, as proper initialization is required for all internal components.
//
// The Context provides isolated key-value storage for each tool execution, enabling
// tools to maintain state and share data during their execution lifecycle.
type Context interface {
	// Context returns the underlying context.Context for cancellation,
	// timeout control, and other standard context operations.
	//
	// This method provides access to the standard Go context for implementing
	// cancellation, deadlines, and carrying chatRequest-scoped values across API boundaries.
	Context() stdContext.Context

	// Set stores a key-value pair in the context's data storage.
	// Empty keys are ignored and will not be stored.
	// Returns the Context instance for method chaining.
	//
	// This method is thread-safe and enables fluent-style programming for
	// accumulating context data across multiple operations.
	//
	// Example:
	//	ctx.Set("userId", 123).Set("sessionId", "abc-123")
	Set(key string, val any) Context

	// SetMap stores multiple key-value pairs from the provided map into the context's data storage.
	// Empty keys in the map are ignored and will not be stored.
	// If a key already exists in the context, its value will be overwritten with the new value.
	// Returns the Context instance for method chaining.
	//
	// This method is thread-safe and provides an efficient way to bulk-load context data
	// while supporting fluent-style programming when combined with other context operations.
	//
	// Example:
	//	data := map[string]any{
	//	    "userId": 123,
	//	    "sessionId": "abc-123",
	//	    "role": "admin",
	//	}
	//	ctx.SetMap(data).Set("timestamp", time.Now())
	SetMap(m map[string]any) Context

	// Get retrieves a value by key from the context's data storage.
	// Returns the value and true if the key exists, otherwise returns nil and false.
	// Empty keys will always return nil and false.
	//
	// This method is thread-safe and follows Go's common pattern of returning both
	// value and existence flag, allowing callers to distinguish between stored nil
	// values and missing keys.
	//
	// Example:
	//	if userId, ok := ctx.Get("userId"); ok {
	//	    // use userId
	//	}
	Get(key string) (any, bool)

	// Fields returns a copy of all fields stored in this context.
	// The returned map is a deep clone, so modifications to it will not
	// affect the original context's fields.
	//
	// This method is thread-safe and useful for debugging, logging, or passing
	// context data to external systems that expect a map interface.
	Fields() map[string]any

	// Clear removes all key-value pairs from the context's data storage.
	// This operation resets the context to a clean state while preserving
	// the underlying context.Context.
	//
	// This method is thread-safe and typically used when reusing context
	// instances across different tool executions or when implementing cleanup operations.
	Clear()

	// Clone creates a deep copy of the Context instance.
	// Returns a new Context with:
	// - The same underlying context.Context
	// - A cloned fields map (independent copy)
	//
	// This ensures that modifications to the clone's fields do not affect
	// the original Context and vice versa.
	//
	// This method is useful for creating branched execution contexts,
	// implementing rollback mechanisms, or creating context snapshots
	// for debugging purposes.
	//
	// Note: The clone operation captures a snapshot of the current state.
	// Changes made to the original context after cloning will not be reflected
	// in the clone.
	Clone() Context
}

// context is the internal implementation of the Context interface.
// It provides thread-safe access to key-value storage through RWMutex synchronization.
type context struct {
	ctx    stdContext.Context
	mu     sync.RWMutex // Protects fields map for concurrent access
	fields map[string]any
}

// NewContext creates a new Context instance with the provided context.Context.
// The returned Context is thread-safe and ready for concurrent use.
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
	if m == nil {
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
	clonedFields := maps.Clone(t.fields)
	t.mu.RUnlock()

	return &context{
		ctx:    t.ctx,
		fields: clonedFields,
	}
}
