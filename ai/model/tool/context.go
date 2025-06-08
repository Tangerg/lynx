package tool

import (
	stdContext "context"
	"maps"
)

// Context represents an execution context for tool operations, providing
// access to contextual data, key-value storage, and message history.
//
// Note: Implementations are NOT required to be thread-safe. They are designed for single-threaded use
// within individual tool execution contexts, where each tool maintains its own isolated Context instance.
// If concurrent access is required, external synchronization must be provided by the caller.
//
// Context instances MUST be created using NewContext() constructor and should not be instantiated
// directly with struct literals, as proper initialization is required for all internal components.
//
// In typical usage patterns, each tool call receives its own Context instance, ensuring isolation
// and eliminating the need for concurrent access patterns.
type Context interface {
	// Context returns the underlying context.Context for cancellation,
	// timeout control, and other standard context operations.
	//
	// This method provides access to the standard Go context for implementing
	// cancellation, deadlines, and carrying request-scoped values across API boundaries.
	Context() stdContext.Context

	// Set stores a key-value pair in the context's data storage.
	// Empty keys are ignored and will not be stored.
	// Returns the Context instance for method chaining.
	//
	// This method enables fluent-style programming and is useful for
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
	// This method provides an efficient way to bulk-load context data and supports
	// fluent-style programming when combined with other context operations.
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
	// This method follows Go's common pattern of returning both value and existence flag,
	// allowing callers to distinguish between stored nil values and missing keys.
	//
	// Example:
	//	if userId, ok := ctx.Get("userId"); ok {
	//	    // use userId
	//	}
	Get(key string) (any, bool)

	// Fields returns a copy of all fields stored in this context.
	// The returned map is a shallow clone, so modifications to the map itself
	// will not affect the original context's fields. However, if the field values
	// are reference types (slices, maps, pointers), modifications to those values
	// may still affect the original data.
	//
	// This method is useful for debugging, logging, or passing context data
	// to external systems that expect a map interface.
	Fields() map[string]any

	// Clear removes all key-value pairs from the context's data storage
	// and clears the message history. This operation resets the context
	// to a clean state while preserving the underlying context.Context.
	//
	// This method is typically used when reusing context instances across
	// different tool executions or when implementing cleanup operations.
	Clear()

	// History returns the message history associated with this context.
	// The returned History instance can be used to add messages, query
	// message content, and manage the conversation history.
	//
	// Note: The returned History is the actual instance, not a copy.
	// Modifications to it will affect this context's history. This design
	// allows for efficient message management without unnecessary copying.
	//
	// Example:
	//	ctx.History().Add(message)
	//	if last, ok := ctx.History().Last(); ok {
	//	    // process last message
	//	}
	History() History

	// Clone creates a deep copy of the Context instance.
	// Returns a new Context with:
	// - The same underlying context.Context
	// - A cloned fields map (independent copy)
	// - A cloned History instance (independent copy)
	//
	// This ensures that modifications to the clone's fields or history
	// do not affect the original Context and vice versa.
	//
	// This method is useful for creating branched execution contexts,
	// implementing rollback mechanisms, or creating context snapshots
	// for debugging purposes.
	Clone() Context
}

// context is the internal implementation of the Context interface.
type context struct {
	ctx     stdContext.Context
	fields  map[string]any
	history History
}

// NewContext creates a new Context instance with the provided context.Context.
func NewContext(ctx stdContext.Context) Context {
	return &context{
		ctx:     ctx,
		fields:  make(map[string]any),
		history: NewHistory(),
	}
}

func (t *context) Context() stdContext.Context {
	return t.ctx
}

func (t *context) Set(key string, val any) Context {
	if key != "" {
		t.fields[key] = val
	}
	return t
}

func (t *context) SetMap(m map[string]any) Context {
	if m != nil {
		for k, v := range m {
			t.Set(k, v)
		}
	}
	return t
}

func (t *context) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	val, ok := t.fields[key]
	return val, ok
}

func (t *context) Fields() map[string]any {
	return maps.Clone(t.fields)
}

func (t *context) Clear() {
	clear(t.fields)
	t.history.Clear()
}

func (t *context) History() History {
	return t.history
}

func (t *context) Clone() Context {
	return &context{
		ctx:     t.ctx,
		fields:  maps.Clone(t.fields),
		history: t.history.Clone(),
	}
}
