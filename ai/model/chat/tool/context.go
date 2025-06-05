package tool

import (
	"context"
	"iter"
	"maps"
)

// Context represents an execution context for tool operations, providing
// access to contextual data, key-value storage, and message history.
//
// Note: This type is NOT thread-safe. It is designed for single-threaded use
// within individual tool execution contexts, where each tool maintains its own
// isolated Context instance. If concurrent access is required, external
// synchronization must be provided by the caller.
//
// Context instances MUST be created using NewContext() constructor and should
// not be instantiated directly with struct literals, as proper initialization
// is required for all internal components.
//
// In typical usage patterns, each tool call receives its own Context instance,
// ensuring isolation and eliminating the need for concurrent access patterns.
type Context struct {
	ctx     context.Context
	field   map[string]any
	history *History
}

// NewContext creates a new Context instance with the provided context.Context.
// This is the only supported way to create a Context - direct struct
// instantiation is not supported and may lead to nil pointer panics.
//
// The returned Context is fully initialized with:
//   - The provided context.Context for cancellation and timeout control
//   - An empty key-value storage map
//   - A new empty History instance
//
// If ctx is nil, it will be stored as-is (callers should typically provide
// context.Background() or a derived context).
func NewContext(ctx context.Context) *Context {
	return &Context{
		ctx:     ctx,
		field:   make(map[string]any),
		history: NewHistory(),
	}
}

// Context returns the underlying context.Context for cancellation,
// timeout control, and other standard context operations.
func (t *Context) Context() context.Context {
	return t.ctx
}

// Set stores a key-value pair in the context's data storage.
// Empty keys are ignored and will not be stored.
// Returns the Context instance for method chaining.
//
// Example:
//
//	ctx.Set("userId", 123).Set("sessionId", "abc-123")
func (t *Context) Set(key string, val any) *Context {
	if key != "" {
		t.field[key] = val
	}
	return t
}

// Get retrieves a value by key from the context's data storage.
// Returns the value and true if the key exists, otherwise returns nil and false.
// Empty keys will always return nil and false.
func (t *Context) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	val, ok := t.field[key]
	return val, ok
}

// Delete removes a key-value pair from the context's data storage.
// Empty keys are ignored. Returns the Context instance for method chaining.
// It is safe to delete non-existent keys.
func (t *Context) Delete(key string) *Context {
	if key != "" {
		delete(t.field, key)
	}
	return t
}

// Iter returns an iterator over all key-value pairs in the context's data storage.
// The iterator yields key-value pairs in an unspecified order.
//
// Example usage:
//
//	for key, value := range ctx.Iter() {
//	    fmt.Printf("Key: %s, Value: %v\n", key, value)
//	}
func (t *Context) Iter() iter.Seq2[string, any] {
	return maps.All(t.field)
}

// Size returns the number of key-value pairs currently stored in the context.
// This count does not include the message history size.
func (t *Context) Size() int {
	return len(t.field)
}

// Clear removes all key-value pairs from the context's data storage
// and clears the message history. This operation resets the context
// to a clean state while preserving the underlying context.Context.
func (t *Context) Clear() {
	clear(t.field)
	t.history.Clear()
}

// SetHistory replaces the current History instance with the provided one.
// Nil history values are ignored to prevent nil pointer panics.
// Returns the Context instance for method chaining.
//
// This method allows sharing history between contexts or restoring
// from a previously saved state.
func (t *Context) SetHistory(history *History) *Context {
	if history != nil {
		t.history = history
	}
	return t
}

// GetHistory returns the message history associated with this context.
// The returned History instance can be used to add messages, query
// message content, and manage the conversation history.
//
// Note: The returned History is the actual instance, not a copy.
// Modifications to it will affect this context's history.
func (t *Context) GetHistory() *History {
	return t.history
}
